package algorithms

import (
	"testing"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

func TestRoundRobin(t *testing.T) {
	servers := []*models.Server{
		{ID: "backend-1", Healthy: true},
		{ID: "backend-2", Healthy: true},
		{ID: "backend-3", Healthy: true},
	}

	rr := &RoundRobin{}

	expected := []string{
		"backend-1",
		"backend-2",
		"backend-3",
		"backend-1",
	}

	for _, expectedID := range expected {
		server := rr.NextServer(servers)

		if server == nil {
			t.Fatal("expected a server, got nil")
		}

		if server.ID != expectedID {
			t.Fatalf("expected %s, got %s", expectedID, server.ID)
		}
	}
}

func TestRoundRobinSkipsUnhealthyServers(t *testing.T) {
	servers := []*models.Server{
		{ID: "backend-1", Healthy: true},
		{ID: "backend-2", Healthy: false},
		{ID: "backend-3", Healthy: true},
	}

	rr := &RoundRobin{}

	expected := []string{
		"backend-1",
		"backend-3",
		"backend-1",
		"backend-3",
	}

	for _, expectedID := range expected {
		server := rr.NextServer(servers)

		if server == nil {
			t.Fatal("expected a server, got nil")
		}

		if server.ID != expectedID {
			t.Fatalf("expected %s, got %s", expectedID, server.ID)
		}
	}
}

func TestRoundRobinNoHealthyServers(t *testing.T) {
	servers := []*models.Server{
		{ID: "backend-1", Healthy: false},
		{ID: "backend-2", Healthy: false},
	}

	rr := &RoundRobin{}

	server := rr.NextServer(servers)

	if server != nil {
		t.Fatalf("expected nil, got %s", server.ID)
	}
}
