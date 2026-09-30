package algorithms

import (
	"testing"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

func TestWeightedRoundRobin(t *testing.T) {
	servers := []*models.Server{
		{ID: "backend-1", Weight: 3, Healthy: true},
		{ID: "backend-2", Weight: 1, Healthy: true},
		{ID: "backend-3", Weight: 2, Healthy: true},
	}

	wrr := &WeightedRoundRobin{}

	expected := []string{
		"backend-1",
		"backend-1",
		"backend-1",
		"backend-2",
		"backend-3",
		"backend-3",
	}

	for _, expectedID := range expected {
		server := wrr.NextServer(servers)

		if server == nil {
			t.Fatal("expected a server, got nil")
		}

		if server.ID != expectedID {
			t.Fatalf("expected %s, got %s", expectedID, server.ID)
		}
	}
}

func TestWeightedRoundRobinEmptyServers(t *testing.T) {
	wrr := &WeightedRoundRobin{}

	server := wrr.NextServer(nil)

	if server != nil {
		t.Fatalf("expected nil, got %v", server)
	}
}

func TestWeightedRoundRobinNoValidServers(t *testing.T) {
	servers := []*models.Server{
		{ID: "backend-1", Weight: 0, Healthy: true},
		{ID: "backend-2", Weight: -1, Healthy: true},
		{ID: "backend-3", Weight: 2, Healthy: false},
	}

	wrr := &WeightedRoundRobin{}

	server := wrr.NextServer(servers)

	if server != nil {
		t.Fatalf("expected nil, got %v", server)
	}
}

func TestWeightedRoundRobinSkipsUnhealthyServer(t *testing.T) {
	servers := []*models.Server{
		{ID: "backend-1", Weight: 3, Healthy: true},
		{ID: "backend-2", Weight: 5, Healthy: false},
		{ID: "backend-3", Weight: 2, Healthy: true},
	}

	wrr := &WeightedRoundRobin{}

	for i := 0; i < 10; i++ {
		server := wrr.NextServer(servers)

		if server == nil {
			t.Fatal("expected a server, got nil")
		}

		if server.ID == "backend-2" {
			t.Fatal("selected unhealthy server")
		}
	}
}
