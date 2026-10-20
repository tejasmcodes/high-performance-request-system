package decisionlog

import (
	"math"
	"testing"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

func TestSnapshotServerCapturesState(t *testing.T) {
	server := models.NewServer("backend-1", "http://localhost:8081", 3)
	server.SetHealthy(false)
	server.IncrementConnections()
	server.IncrementConnections()
	server.ObserveResponseTime(25 * time.Millisecond)
	server.ObserveResponseTime(75 * time.Millisecond)
	server.SetCPUUtilization(42.5)

	now := server.CPUUpdatedAt().Add(150 * time.Millisecond)
	snapshot := SnapshotServer(server, now)

	if snapshot.BackendID != "backend-1" {
		t.Fatalf("expected backend-1, got %s", snapshot.BackendID)
	}
	if snapshot.Healthy {
		t.Fatal("expected unhealthy backend")
	}
	if snapshot.Weight != 3 {
		t.Fatalf("expected weight 3, got %d", snapshot.Weight)
	}
	if snapshot.ActiveConnections != 2 {
		t.Fatalf("expected 2 active connections, got %d", snapshot.ActiveConnections)
	}
	if snapshot.ResponseTimeSamples != 2 {
		t.Fatalf("expected 2 response time samples, got %d", snapshot.ResponseTimeSamples)
	}
	if !almostEqual(snapshot.ResponseTimeEMAMs, 35) {
		t.Fatalf("expected response time EMA 35, got %f", snapshot.ResponseTimeEMAMs)
	}
	if snapshot.CPUUtilization != 42.5 {
		t.Fatalf("expected CPU utilization 42.5, got %f", snapshot.CPUUtilization)
	}
	if !snapshot.CPUUpdatedAt.Equal(server.CPUUpdatedAt()) {
		t.Fatalf("expected CPU updated at %v, got %v", server.CPUUpdatedAt(), snapshot.CPUUpdatedAt)
	}
	if snapshot.CPUAgeMs != 150 {
		t.Fatalf("expected CPU age 150ms, got %d", snapshot.CPUAgeMs)
	}
}

func TestSnapshotServerMissingResponseTimeSamples(t *testing.T) {
	server := models.NewServer("backend-1", "http://localhost:8081", 1)

	snapshot := SnapshotServer(server, time.Now())

	if snapshot.ResponseTimeSamples != 0 {
		t.Fatalf("expected 0 response time samples, got %d", snapshot.ResponseTimeSamples)
	}
	if snapshot.ResponseTimeEMAMs != 0 {
		t.Fatalf("expected zero response time EMA, got %f", snapshot.ResponseTimeEMAMs)
	}
}

func TestSnapshotServerZeroCPUUpdatedAt(t *testing.T) {
	server := models.NewServer("backend-1", "http://localhost:8081", 1)

	snapshot := SnapshotServer(server, time.Now())

	if !snapshot.CPUUpdatedAt.IsZero() {
		t.Fatalf("expected zero CPU updated at, got %v", snapshot.CPUUpdatedAt)
	}
	if snapshot.CPUAgeMs != 0 {
		t.Fatalf("expected zero CPU age, got %d", snapshot.CPUAgeMs)
	}
}

func TestSnapshotServersPreservesOrderAndIncludesAllBackends(t *testing.T) {
	servers := []*models.Server{
		models.NewServer("backend-1", "http://localhost:8081", 3),
		models.NewServer("backend-2", "http://localhost:8082", 0),
		models.NewServer("backend-3", "http://localhost:8083", 2),
	}

	servers[0].SetHealthy(false)
	servers[1].IncrementConnections()

	snapshots := SnapshotServers(servers, time.Now())
	if len(snapshots) != len(servers) {
		t.Fatalf("expected %d snapshots, got %d", len(servers), len(snapshots))
	}

	for i, server := range servers {
		if snapshots[i].BackendID != server.ID {
			t.Fatalf("expected snapshot %d to be %s, got %s", i, server.ID, snapshots[i].BackendID)
		}
	}

	if snapshots[0].Healthy {
		t.Fatal("expected backend-1 to be unhealthy in snapshot")
	}
	if snapshots[1].Weight != 0 {
		t.Fatalf("expected zero-weight backend to be included, got weight %d", snapshots[1].Weight)
	}
	if snapshots[1].ActiveConnections != 1 {
		t.Fatalf("expected 1 active connection, got %d", snapshots[1].ActiveConnections)
	}
}

func TestSnapshotServersEmptyInput(t *testing.T) {
	snapshots := SnapshotServers(nil, time.Now())

	if snapshots == nil {
		t.Fatal("expected non-nil empty snapshot slice")
	}
	if len(snapshots) != 0 {
		t.Fatalf("expected 0 snapshots, got %d", len(snapshots))
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
