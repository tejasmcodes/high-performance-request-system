package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

func newMetricsServer(t *testing.T, body string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cpuMetricsPath {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func TestCPUCollectorCollect(t *testing.T) {
	testServer := newMetricsServer(t, `{"cpu_utilization": 42.5}`)
	defer testServer.Close()

	server := models.NewServer("backend-1", testServer.URL, 1)
	collector := NewCPUCollector([]*models.Server{server})

	if err := collector.collect(context.Background(), server); err != nil {
		t.Fatalf("collect: %v", err)
	}

	if got := server.CPUUtilization(); got != 42.5 {
		t.Fatalf("expected 42.5, got %f", got)
	}
}

func TestCPUCollectorInvalidJSON(t *testing.T) {
	testServer := newMetricsServer(t, `not json`)
	defer testServer.Close()

	server := models.NewServer("backend-1", testServer.URL, 1)
	collector := NewCPUCollector([]*models.Server{server})

	if err := collector.collect(context.Background(), server); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestCPUCollectorUnexpectedStatus(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer testServer.Close()

	server := models.NewServer("backend-1", testServer.URL, 1)
	collector := NewCPUCollector([]*models.Server{server})

	if err := collector.collect(context.Background(), server); err == nil {
		t.Fatal("expected status error")
	}
}

func TestCPUCollectorInvalidValues(t *testing.T) {
	tests := []string{
		`{"cpu_utilization": -1}`,
		`{"cpu_utilization": 101}`,
	}

	for _, body := range tests {
		testServer := newMetricsServer(t, body)
		server := models.NewServer("backend-1", testServer.URL, 1)
		collector := NewCPUCollector([]*models.Server{server})

		if err := collector.collect(context.Background(), server); err == nil {
			t.Fatalf("expected validation error for %s", body)
		}

		if got := server.CPUUtilization(); got != 0 {
			t.Fatalf("expected CPU value to remain 0, got %f", got)
		}

		testServer.Close()
	}
}

func TestCPUCollectorPreservesValueOnFailure(t *testing.T) {
	testServer := newMetricsServer(t, `{"cpu_utilization": 25}`)
	defer testServer.Close()

	server := models.NewServer("backend-1", testServer.URL, 1)
	collector := NewCPUCollector([]*models.Server{server})

	if err := collector.collect(context.Background(), server); err != nil {
		t.Fatalf("collect: %v", err)
	}

	testServer.Close()

	if err := collector.collect(context.Background(), server); err == nil {
		t.Fatal("expected collection error")
	}

	if got := server.CPUUtilization(); got != 25 {
		t.Fatalf("expected previous value 25, got %f", got)
	}
}

func TestCPUCollectorUnreachableBackend(t *testing.T) {
	server := models.NewServer("backend-1", "http://127.0.0.1:0", 1)
	collector := NewCPUCollector([]*models.Server{server})

	if err := collector.collect(context.Background(), server); err == nil {
		t.Fatal("expected connection error")
	}
}

func TestCPUCollectorCollectAll(t *testing.T) {
	testServer := newMetricsServer(t, `{"cpu_utilization": 12}`)
	defer testServer.Close()

	servers := []*models.Server{
		models.NewServer("backend-1", testServer.URL, 1),
		models.NewServer("backend-2", testServer.URL, 1),
		models.NewServer("backend-3", "http://127.0.0.1:0", 1),
	}

	collector := NewCPUCollector(servers)
	collector.CollectAll(context.Background())

	for _, server := range servers[:2] {
		if got := server.CPUUtilization(); got != 12 {
			t.Fatalf("%s: expected 12, got %f", server.ID, got)
		}
	}
}

func TestCPUCollectorContextCancellation(t *testing.T) {
	testServer := newMetricsServer(t, `{"cpu_utilization": 12}`)
	defer testServer.Close()

	server := models.NewServer("backend-1", testServer.URL, 1)
	collector := NewCPUCollector([]*models.Server{server})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := collector.collect(ctx, server); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestCPUCollectorStartStopsOnContextCancel(t *testing.T) {
	testServer := newMetricsServer(t, `{"cpu_utilization": 12}`)
	defer testServer.Close()

	server := models.NewServer("backend-1", testServer.URL, 1)
	collector := NewCPUCollector([]*models.Server{server})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		collector.Start(ctx, 10*time.Millisecond)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("collector did not stop after context cancellation")
	}
}
