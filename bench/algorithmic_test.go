package bench

import (
	"testing"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/algorithms"
	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

type strategyCase struct {
	name string
	new  func() algorithms.Strategy
}

func strategyCases() []strategyCase {
	return []strategyCase{
		{name: "round-robin", new: func() algorithms.Strategy { return &algorithms.RoundRobin{} }},
		{name: "least-connections", new: func() algorithms.Strategy { return &algorithms.LeastConnections{} }},
		{name: "weighted-round-robin", new: func() algorithms.Strategy { return &algorithms.WeightedRoundRobin{} }},
		{name: "adaptive", new: func() algorithms.Strategy { return algorithms.NewAdaptive(algorithms.DefaultAdaptiveConfig()) }},
	}
}

func newBenchmarkServers() []*models.Server {
	servers := []*models.Server{
		models.NewServer("backend-1", "http://127.0.0.1:18081", 3),
		models.NewServer("backend-2", "http://127.0.0.1:18082", 1),
		models.NewServer("backend-3", "http://127.0.0.1:18083", 2),
	}

	servers[0].IncrementConnections()
	servers[0].IncrementConnections()
	servers[1].IncrementConnections()

	servers[1].ObserveResponseTime(50 * time.Millisecond)
	servers[2].ObserveResponseTime(125 * time.Millisecond)

	servers[2].SetCPUUtilization(70)

	return servers
}

func BenchmarkStrategies(b *testing.B) {
	for _, tc := range strategyCases() {
		b.Run(tc.name, func(b *testing.B) {
			servers := newBenchmarkServers()
			strategy := tc.new()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = strategy.NextServer(servers)
			}
		})
	}
}

func TestStrategiesExcludeUnhealthyServers(t *testing.T) {
	for _, tc := range strategyCases() {
		t.Run(tc.name, func(t *testing.T) {
			servers := newBenchmarkServers()
			servers[0].SetHealthy(false)
			servers[1].SetHealthy(false)

			strategy := tc.new()

			for i := 0; i < 20; i++ {
				selected := strategy.NextServer(servers)
				if selected == nil {
					t.Fatal("expected a server, got nil")
				}
				if selected.ID != "backend-3" {
					t.Fatalf("expected backend-3, got %s", selected.ID)
				}
			}
		})
	}
}

func TestStrategiesReturnNilWithNoEligibleServers(t *testing.T) {
	for _, tc := range strategyCases() {
		t.Run(tc.name, func(t *testing.T) {
			servers := newBenchmarkServers()
			for _, server := range servers {
				server.SetHealthy(false)
			}

			strategy := tc.new()

			if selected := strategy.NextServer(servers); selected != nil {
				t.Fatalf("expected nil, got %s", selected.ID)
			}
		})
	}
}

func TestWeightedRoundRobinUsesConfiguredWeights(t *testing.T) {
	servers := newBenchmarkServers()
	strategy := &algorithms.WeightedRoundRobin{}

	counts := map[string]int{}
	for i := 0; i < 6; i++ {
		selected := strategy.NextServer(servers)
		if selected == nil {
			t.Fatal("expected a server, got nil")
		}
		counts[selected.ID]++
	}

	expected := map[string]int{
		"backend-1": 3,
		"backend-2": 1,
		"backend-3": 2,
	}

	for id, count := range expected {
		if counts[id] != count {
			t.Fatalf("expected %s to receive %d requests, got %d", id, count, counts[id])
		}
	}
}
