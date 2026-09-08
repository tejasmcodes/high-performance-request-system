	package main

	import (
		"log"
		"net/http"
		"net/http/httputil"
		"net/url"
		"context"

		"github.com/tejasmcodes/high-performance-request-system/internal/health"
		"github.com/tejasmcodes/high-performance-request-system/internal/algorithms"
		"github.com/tejasmcodes/high-performance-request-system/internal/models"
	)

	func main() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		servers := []*models.Server{
			{ID: "backend-1", URL: "http://localhost:8081", Weight: 3, Healthy: true},
			{ID: "backend-2", URL: "http://localhost:8082", Weight: 1, Healthy: true},
			{ID: "backend-3", URL: "http://localhost:8083", Weight: 2, Healthy: true},
		}

		checker := health.NewHealthChecker(servers)
		go checker.Start(ctx)
		

		strategy := &algorithms.WeightedRoundRobin{}

		handler := func(w http.ResponseWriter, r *http.Request) {
			server := strategy.NextServer(servers)

			if server == nil {
				http.Error(w, "no healthy backend available", http.StatusServiceUnavailable)
				return
			}

			server.IncrementConnections()
			defer server.DecrementConnections()

			target, err := url.Parse(server.URL)
			if err != nil {
				http.Error(w, "invalid backend URL", http.StatusInternalServerError)
				return
			}

			proxy := httputil.NewSingleHostReverseProxy(target)
			proxy.ServeHTTP(w, r)
		}

		log.Println("load balancer listening on :8080")

		if err := http.ListenAndServe(":8080", http.HandlerFunc(handler)); err != nil {
			log.Fatal(err)
		}
	}