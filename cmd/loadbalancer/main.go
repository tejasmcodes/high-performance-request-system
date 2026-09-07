package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/tejasmcodes/high-performance-request-system/internal/algorithms"
	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

func main() {
	servers := []*models.Server{
		{ID: "backend-1", URL: "http://localhost:8081", Healthy: true},
		{ID: "backend-2", URL: "http://localhost:8082", Healthy: true},
		{ID: "backend-3", URL: "http://localhost:8083", Healthy: true},
	}

	strategy := &algorithms.RoundRobin{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		server := strategy.NextServer(servers)

		if server == nil {
			http.Error(w, "no healthy backend available", http.StatusServiceUnavailable)
			return
		}

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