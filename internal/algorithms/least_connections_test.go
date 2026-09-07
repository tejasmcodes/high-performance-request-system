package algorithms

import (
	"testing"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

func TestLeastConnections(t *testing.T) {
	servers := []*models.Server{
		{ID: "backend-1", Healthy: true},
		{ID: "backend-2", Healthy: true},
		{ID: "backend-3", Healthy: true},
	}

	// Give the servers different active connection counts.
	servers[0].IncrementConnections()
	servers[0].IncrementConnections()

	servers[1].IncrementConnections()

	lc := &LeastConnections{}

	server := lc.NextServer(servers)

	if server == nil {
		t.Fatal("expected a server, got nil")
	}

	if server.ID != "backend-3" {
		t.Fatalf("expected backend-3, got %s", server.ID)
	}
}

func TestLeastConnectionsSkipsUnhealthyServers(t *testing.T) {
	servers := []*models.Server{
		{ID: "backend-1", Healthy: true},
		{ID: "backend-2", Healthy: false},
		{ID: "backend-3", Healthy: true},
	}

	servers[0].IncrementConnections()
	servers[0].IncrementConnections()

	lc := &LeastConnections{}

	server := lc.NextServer(servers)

	if server == nil {
		t.Fatal("expected a server, got nil")
	}

	if server.ID != "backend-3" {
		t.Fatalf("expected backend-3, got %s", server.ID)
	}
}

func TestLeastConnectionsNoHealthyServers(t *testing.T) {
	servers := []*models.Server{
		{ID: "backend-1", Healthy: false},
		{ID: "backend-2", Healthy: false},
	}

	lc := &LeastConnections{}

	server := lc.NextServer(servers)

	if server != nil {
		t.Fatalf("expected nil, got %s", server.ID)
	}
}

func TestLeastConnectionsEmptyServers(t *testing.T) {
	lc := &LeastConnections{}

	server := lc.NextServer(nil)

	if server != nil {
		t.Fatalf("expected nil, got %s", server.ID)
	}
}
