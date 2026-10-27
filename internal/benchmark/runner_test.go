package benchmark

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunUsesExistingWorkloadAndRoutesToConfiguredBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/metrics/cpu":
			_, _ = fmt.Fprint(w, `{"cpu_utilization":0}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer backend.Close()

	result, err := Run(context.Background(), Config{
		Algorithm:      "round-robin",
		Scenario:       "baseline",
		Servers:        []ServerConfig{{ID: "backend-test", URL: backend.URL, Weight: 1}},
		Duration:       100 * time.Millisecond,
		Concurrency:    2,
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests == 0 || result.TotalRequests != result.Latency.Count {
		t.Fatalf("expected measured requests and latency samples: %+v", result)
	}
	if result.Backends["backend-test"] != result.TotalRequests {
		t.Fatalf("expected backend distribution, got %+v", result.Backends)
	}
}
