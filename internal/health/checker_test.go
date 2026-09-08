package health

import (
	"testing"
	"net/http"
	"time"
	"net/http/httptest"
	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

func TestCheckServerHealthy(t *testing.T) {
	server := &models.Server{
		ID:      "backend-1",
		URL:     "http://localhost:8081",
		Healthy: false,
	}

	checker := NewHealthChecker([]*models.Server{server})

	checker.checkServer(server)
	if !server.IsHealthy() {
		t.Fatalf("expected server to be healthy")
	}
}
func TestCheckServerUnhealthy(t *testing.T) {
	server := &models.Server{
		ID:      "backend-1",
		URL:     "http://localhost:9999",
		Healthy: true,
	}

	checker := NewHealthChecker([]*models.Server{server})

	checker.checkServer(server)

	if server.IsHealthy() {
		t.Fatal("expected server to be unhealthy")
	}
}


func TestCheckAllConcurrent(t *testing.T) {
    testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(1 * time.Second)
        w.WriteHeader(http.StatusOK)
    }))
    defer testServer.Close()

    servers := []*models.Server{
		{
			ID:      "backend-1",
			URL:     testServer.URL,
			Healthy: false,
		},
		{
			ID:      "backend-2",
			URL:     testServer.URL,
			Healthy: false,
		},
		{
			ID:      "backend-3",
			URL:     testServer.URL,
			Healthy: false,
		},
	}

	checker := NewHealthChecker(servers)
	start := time.Now()
	checker.CheckAll()
	elapsed := time.Since(start)

	t.Logf("checkAll too %v",elapsed)
}