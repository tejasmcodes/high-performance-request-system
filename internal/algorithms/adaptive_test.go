package algorithms

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

func TestDefaultAdaptiveConfig(t *testing.T) {
	cfg := DefaultAdaptiveConfig()

	if cfg.ConnectionsWeight != 0.45 {
		t.Fatalf("expected connections weight 0.45, got %f", cfg.ConnectionsWeight)
	}
	if cfg.ResponseTimeWeight != 0.40 {
		t.Fatalf("expected response time weight 0.40, got %f", cfg.ResponseTimeWeight)
	}
	if cfg.CPUWeight != 0.15 {
		t.Fatalf("expected CPU weight 0.15, got %f", cfg.CPUWeight)
	}
	if cfg.MissingMetricScore != 0.5 {
		t.Fatalf("expected missing metric score 0.5, got %f", cfg.MissingMetricScore)
	}
	if cfg.CPUStaleAfter != 15*time.Second {
		t.Fatalf("expected CPU stale threshold 15s, got %v", cfg.CPUStaleAfter)
	}
	if cfg.ResponseTimeStaleAfter != 15*time.Second {
		t.Fatalf("expected response time stale threshold 15s, got %v", cfg.ResponseTimeStaleAfter)
	}
	if cfg.ScoreEpsilon != 1e-6 {
		t.Fatalf("expected score epsilon 1e-6, got %g", cfg.ScoreEpsilon)
	}
}

func TestAdaptiveSelectsFewestConnections(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
		newAdaptiveServer("backend-3", 1),
	}

	setConnections(servers[0], 2)
	setConnections(servers[1], 1)

	adaptive := NewAdaptive(DefaultAdaptiveConfig())

	selected := adaptive.NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-3" {
		t.Fatalf("expected backend-3, got %s", selected.ID)
	}
}

func TestAdaptiveCapacityAwareConnectionScores(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 3),
		newAdaptiveServer("backend-2", 1),
		newAdaptiveServer("backend-3", 2),
	}

	setConnections(servers[0], 6)
	setConnections(servers[1], 3)
	setConnections(servers[2], 2)

	if got := capacityAdjustedConnections(6, 3); got != 2 {
		t.Fatalf("expected backend-1 effective connections 2, got %f", got)
	}
	if got := capacityAdjustedConnections(3, 1); got != 3 {
		t.Fatalf("expected backend-2 effective connections 3, got %f", got)
	}
	if got := capacityAdjustedConnections(2, 2); got != 1 {
		t.Fatalf("expected backend-3 effective connections 1, got %f", got)
	}

	cfg := DefaultAdaptiveConfig()
	cfg.ResponseTimeWeight = 0
	cfg.CPUWeight = 0

	selected := NewAdaptive(cfg).NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-3" {
		t.Fatalf("expected backend-3 (lowest effective connections), got %s", selected.ID)
	}
}

func TestAdaptiveEqualEffectiveConnectionsHaveZeroScore(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 3),
		newAdaptiveServer("backend-2", 1),
		newAdaptiveServer("backend-3", 2),
	}

	setConnections(servers[0], 3)
	setConnections(servers[1], 1)
	setConnections(servers[2], 2)

	effectiveConnections := []float64{
		capacityAdjustedConnections(3, 3),
		capacityAdjustedConnections(1, 1),
		capacityAdjustedConnections(2, 2),
	}
	for _, connections := range effectiveConnections {
		if score := normalizeMinMax(connections, 1, 1); score != 0 {
			t.Fatalf("expected zero connection score, got %f", score)
		}
	}

	cfg := DefaultAdaptiveConfig()
	cfg.ResponseTimeWeight = 0
	cfg.CPUWeight = 0

	selected := NewAdaptive(cfg).NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-1" {
		t.Fatalf("expected tie-breaking to choose backend-1, got %s", selected.ID)
	}
}

func TestAdaptiveSelectsLowestResponseTime(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
		newAdaptiveServer("backend-3", 1),
	}

	setResponseTime(servers[0], 10)
	setResponseTime(servers[1], 30)
	setResponseTime(servers[2], 20)

	adaptive := NewAdaptive(DefaultAdaptiveConfig())

	selected := adaptive.NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-1" {
		t.Fatalf("expected backend-1, got %s", selected.ID)
	}
}

func TestAdaptiveSelectsLowestCPU(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
		newAdaptiveServer("backend-3", 1),
	}

	servers[0].SetCPUUtilization(90)
	servers[1].SetCPUUtilization(10)
	servers[2].SetCPUUtilization(50)

	adaptive := NewAdaptive(DefaultAdaptiveConfig())

	selected := adaptive.NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-2" {
		t.Fatalf("expected backend-2, got %s", selected.ID)
	}
}

func TestAdaptiveExcludesUnhealthyServers(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
		newAdaptiveServer("backend-3", 1),
	}

	servers[0].SetHealthy(false)
	servers[1].SetHealthy(false)

	adaptive := NewAdaptive(DefaultAdaptiveConfig())

	selected := adaptive.NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-3" {
		t.Fatalf("expected backend-3, got %s", selected.ID)
	}
}

func TestAdaptiveExcludesDisabledServers(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 0),
		newAdaptiveServer("backend-2", -1),
		newAdaptiveServer("backend-3", 2),
	}

	setConnections(servers[0], 0)

	adaptive := NewAdaptive(DefaultAdaptiveConfig())

	selected := adaptive.NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-3" {
		t.Fatalf("expected backend-3, got %s", selected.ID)
	}
}

func TestAdaptiveNoEligibleServers(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 0),
		newAdaptiveServer("backend-2", 1),
	}

	servers[1].SetHealthy(false)

	adaptive := NewAdaptive(DefaultAdaptiveConfig())

	if selected := adaptive.NextServer(servers); selected != nil {
		t.Fatalf("expected nil, got %s", selected.ID)
	}
}

func TestAdaptiveEmptyServers(t *testing.T) {
	adaptive := NewAdaptive(DefaultAdaptiveConfig())

	if selected := adaptive.NextServer(nil); selected != nil {
		t.Fatalf("expected nil, got %s", selected.ID)
	}
}

func TestAdaptiveMissingResponseTimeIsNotPreferred(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
	}

	setResponseTime(servers[1], 100)

	adaptive := NewAdaptive(DefaultAdaptiveConfig())

	selected := adaptive.NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-2" {
		t.Fatalf("expected backend-2, got %s", selected.ID)
	}
}

func TestAdaptiveResponseTimeNormalizationIgnoresMissing(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
		newAdaptiveServer("backend-3", 1),
	}

	setResponseTime(servers[0], 10)
	setResponseTime(servers[2], 20)

	adaptive := NewAdaptive(DefaultAdaptiveConfig())

	selected := adaptive.NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-1" {
		t.Fatalf("expected backend-1, got %s", selected.ID)
	}
}

func TestAdaptiveFreshResponseTimeParticipatesInScoring(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
	}

	setResponseTime(servers[0], 10)
	setResponseTime(servers[1], 20)

	cfg := DefaultAdaptiveConfig()
	cfg.ConnectionsWeight = 0
	cfg.ResponseTimeWeight = 1
	cfg.CPUWeight = 0

	selected := NewAdaptive(cfg).NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-1" {
		t.Fatalf("expected backend-1 with lower fresh response time, got %s", selected.ID)
	}
}

func TestAdaptiveStaleResponseTimeUsesMissingScore(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
	}

	setResponseTime(servers[0], 10)

	cfg := DefaultAdaptiveConfig()
	cfg.ConnectionsWeight = 0
	cfg.ResponseTimeWeight = 1
	cfg.CPUWeight = 0
	cfg.ResponseTimeStaleAfter = 20 * time.Millisecond

	time.Sleep(50 * time.Millisecond)
	setResponseTime(servers[1], 20)

	selected := NewAdaptive(cfg).NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-2" {
		t.Fatalf("expected backend-2 with fresh response time, got %s", selected.ID)
	}
}

func TestAdaptiveMissingResponseTimeUsesMissingScore(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
	}

	_, samples, updatedAt := servers[0].ResponseTimeSnapshot()
	if samples != 0 || !updatedAt.IsZero() {
		t.Fatalf("expected missing response time state, got samples=%d updatedAt=%v", samples, updatedAt)
	}
	setResponseTime(servers[1], 100)

	cfg := DefaultAdaptiveConfig()
	cfg.ConnectionsWeight = 0
	cfg.ResponseTimeWeight = 1
	cfg.CPUWeight = 0

	selected := NewAdaptive(cfg).NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-2" {
		t.Fatalf("expected backend-2 with fresh response time, got %s", selected.ID)
	}
}

func TestAdaptiveStaleCPUUsesMissingScore(t *testing.T) {
	cfg := DefaultAdaptiveConfig()
	cfg.CPUStaleAfter = 20 * time.Millisecond

	servers := []*models.Server{
		newAdaptiveServer("backend-1", 2),
		newAdaptiveServer("backend-2", 1),
		newAdaptiveServer("backend-3", 1),
	}

	servers[0].SetCPUUtilization(0)

	time.Sleep(50 * time.Millisecond)

	servers[2].SetCPUUtilization(0)

	adaptive := NewAdaptive(cfg)

	selected := adaptive.NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-3" {
		t.Fatalf("expected backend-3 (fresh CPU), got %s", selected.ID)
	}
}

func TestAdaptiveInvalidCPUUsesMissingScore(t *testing.T) {
	tests := []struct {
		name  string
		value float64
	}{
		{name: "above range", value: 150},
		{name: "below range", value: -1},
		{name: "nan", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			servers := []*models.Server{
				newAdaptiveServer("backend-1", 1),
				newAdaptiveServer("backend-2", 2),
			}

			servers[0].SetCPUUtilization(tt.value)

			adaptive := NewAdaptive(DefaultAdaptiveConfig())

			selected := adaptive.NextServer(servers)
			if selected == nil {
				t.Fatal("expected a server, got nil")
			}
			if selected.ID != "backend-2" {
				t.Fatalf("expected backend-2, got %s", selected.ID)
			}
		})
	}
}

func TestAdaptiveTiePrefersHigherWeight(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 3),
		newAdaptiveServer("backend-2", 1),
		newAdaptiveServer("backend-3", 2),
	}

	adaptive := NewAdaptive(DefaultAdaptiveConfig())

	for i := 0; i < 5; i++ {
		selected := adaptive.NextServer(servers)
		if selected == nil {
			t.Fatal("expected a server, got nil")
		}
		if selected.ID != "backend-1" {
			t.Fatalf("expected backend-1, got %s", selected.ID)
		}
	}
}

func TestAdaptiveTieRotatesEqualWeights(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
		newAdaptiveServer("backend-3", 1),
	}

	adaptive := NewAdaptive(DefaultAdaptiveConfig())

	expected := []string{
		"backend-1",
		"backend-2",
		"backend-3",
		"backend-1",
	}

	for _, id := range expected {
		selected := adaptive.NextServer(servers)
		if selected == nil {
			t.Fatal("expected a server, got nil")
		}
		if selected.ID != id {
			t.Fatalf("expected %s, got %s", id, selected.ID)
		}
	}
}

func TestAdaptiveScoreEpsilonTieBand(t *testing.T) {
	cfg := DefaultAdaptiveConfig()
	cfg.ConnectionsWeight = 5e-7
	cfg.ResponseTimeWeight = 0
	cfg.CPUWeight = 0
	cfg.ScoreEpsilon = 1e-6

	servers := []*models.Server{
		newAdaptiveServer("backend-1", 2),
		newAdaptiveServer("backend-2", 1),
	}

	setConnections(servers[0], 1)

	adaptive := NewAdaptive(cfg)

	selected := adaptive.NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-1" {
		t.Fatalf("expected tie to prefer backend-1, got %s", selected.ID)
	}
}

func TestAdaptiveScoreOutsideEpsilon(t *testing.T) {
	cfg := DefaultAdaptiveConfig()
	cfg.ConnectionsWeight = 5e-7
	cfg.ResponseTimeWeight = 0
	cfg.CPUWeight = 0
	cfg.ScoreEpsilon = 1e-9

	servers := []*models.Server{
		newAdaptiveServer("backend-1", 2),
		newAdaptiveServer("backend-2", 1),
	}

	setConnections(servers[0], 1)

	adaptive := NewAdaptive(cfg)

	selected := adaptive.NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-2" {
		t.Fatalf("expected backend-2 (lower score), got %s", selected.ID)
	}
}

func TestAdaptiveZeroCoefficientDisablesSignal(t *testing.T) {
	cfg := DefaultAdaptiveConfig()
	cfg.ConnectionsWeight = 0
	cfg.ResponseTimeWeight = 1
	cfg.CPUWeight = 0

	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
	}

	setConnections(servers[0], 10)
	setResponseTime(servers[0], 10)
	setResponseTime(servers[1], 50)

	adaptive := NewAdaptive(cfg)

	selected := adaptive.NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-1" {
		t.Fatalf("expected backend-1, got %s", selected.ID)
	}
}

func TestAdaptiveZeroWeightsFallBackToDefaults(t *testing.T) {
	cfg := AdaptiveConfig{}

	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
		newAdaptiveServer("backend-3", 1),
	}

	setConnections(servers[0], 2)
	setConnections(servers[1], 1)

	adaptive := NewAdaptive(cfg)

	selected := adaptive.NextServer(servers)
	if selected == nil {
		t.Fatal("expected a server, got nil")
	}
	if selected.ID != "backend-3" {
		t.Fatalf("expected backend-3, got %s", selected.ID)
	}
}

func TestAdaptiveConcurrentAccess(t *testing.T) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
		newAdaptiveServer("backend-3", 1),
	}

	adaptive := NewAdaptive(DefaultAdaptiveConfig())

	var wg sync.WaitGroup

	for worker := 0; worker < 8; worker++ {
		wg.Add(1)

		go func(worker int) {
			defer wg.Done()

			server := servers[worker%len(servers)]

			for i := 0; i < 200; i++ {
				if selected := adaptive.NextServer(servers); selected == nil {
					t.Error("expected a server, got nil")
					return
				}

				server.IncrementConnections()
				server.ObserveResponseTime(time.Duration(i%50+1) * time.Millisecond)
				server.SetCPUUtilization(float64(i % 101))
				server.DecrementConnections()
			}
		}(worker)
	}

	wg.Wait()
}

func BenchmarkAdaptiveNextServer(b *testing.B) {
	servers := []*models.Server{
		newAdaptiveServer("backend-1", 1),
		newAdaptiveServer("backend-2", 1),
		newAdaptiveServer("backend-3", 1),
	}

	setConnections(servers[0], 2)
	servers[1].ObserveResponseTime(25 * time.Millisecond)
	servers[2].SetCPUUtilization(40)

	adaptive := NewAdaptive(DefaultAdaptiveConfig())

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = adaptive.NextServer(servers)
	}
}

func newAdaptiveServer(id string, weight int) *models.Server {
	return models.NewServer(id, "http://"+id, weight)
}

func setConnections(server *models.Server, count int) {
	for i := 0; i < count; i++ {
		server.IncrementConnections()
	}
}

func setResponseTime(server *models.Server, milliseconds int) {
	server.ObserveResponseTime(time.Duration(milliseconds) * time.Millisecond)
}
