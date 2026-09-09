package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/algorithms"
	"github.com/tejasmcodes/high-performance-request-system/internal/health"
	"github.com/tejasmcodes/high-performance-request-system/internal/models"
	"github.com/tejasmcodes/high-performance-request-system/internal/metrics"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	servers := []*models.Server{
		models.NewServer("backend-1", "http://localhost:8081", 3),
		models.NewServer("backend-2", "http://localhost:8082", 1),
		models.NewServer("backend-3", "http://localhost:8083", 2),
		}

	checker := health.NewHealthChecker(servers)
	go checker.Start(ctx)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			cpu, err := metrics.CPUUtilization()
			if err != nil {
				log.Printf("CPU utilization error: %v", err)
				continue
			}

			log.Printf("CPU utilization: %.2f%%", cpu)
		}
	}()

	strategy := &algorithms.WeightedRoundRobin{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		server := strategy.NextServer(servers)

		if server == nil {
			http.Error(w, "no healthy backend available", http.StatusServiceUnavailable)
			return
		}

		log.Printf("selected backend=%s", server.ID)

		server.IncrementConnections()
		defer server.DecrementConnections()

		target, err := url.Parse(server.URL)
		if err != nil {
			http.Error(w, "invalid backend URL", http.StatusInternalServerError)
			return
		}

		proxy := httputil.NewSingleHostReverseProxy(target)

		start := time.Now()

		proxy.ServeHTTP(w, r)

		elapsed := time.Since(start)
		server.ObserveResponseTime(elapsed)
		log.Printf(
			"backend=%s response_time=%.2fms",
			server.ID,
			server.ResponseTime(),
		)
	}

	log.Println("load balancer listening on :8080")

	if err := http.ListenAndServe(":8080", http.HandlerFunc(handler)); err != nil {
		log.Fatal(err)
	}
}
