package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/algorithms"
	"github.com/tejasmcodes/high-performance-request-system/internal/health"
	"github.com/tejasmcodes/high-performance-request-system/internal/models"
	"github.com/tejasmcodes/high-performance-request-system/internal/monitor"
)

type selectionStats struct {
	mu     sync.Mutex
	counts map[string]uint64
	misses uint64
}

func newSelectionStats(ids []string) *selectionStats {
	counts := make(map[string]uint64, len(ids))
	for _, id := range ids {
		counts[id] = 0
	}

	return &selectionStats{counts: counts}
}

func (s *selectionStats) observe(server *models.Server) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if server == nil {
		s.misses++
		return
	}

	s.counts[server.ID]++
}

func (s *selectionStats) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	counts := make(map[string]uint64, len(s.counts))
	for id, count := range s.counts {
		counts[id] = count
	}

	return map[string]any{
		"counts": counts,
		"misses": s.misses,
	}
}

type countingStrategy struct {
	inner algorithms.Strategy
	stats *selectionStats
}

func (c *countingStrategy) NextServer(servers []*models.Server) *models.Server {
	server := c.inner.NextServer(servers)
	c.stats.observe(server)
	return server
}

func main() {
	addr := flag.String("addr", ":18080", "load balancer listen address")
	algorithm := flag.String("algorithm", "adaptive", "routing strategy")
	idsFlag := flag.String("ids", "backend-1,backend-2,backend-3", "comma-separated backend ids")
	urlsFlag := flag.String("urls", "http://localhost:18081,http://localhost:18082,http://localhost:18083", "comma-separated backend URLs")
	weightsFlag := flag.String("weights", "3,1,2", "comma-separated static weights")
	verbose := flag.Bool("verbose", false, "enable request logging")
	flag.Parse()

	if !*verbose {
		log.SetOutput(discard{})
	}

	ids := splitCSV(*idsFlag)
	urls := splitCSV(*urlsFlag)
	weights := parseWeights(*weightsFlag)

	if len(ids) != len(urls) || len(ids) != len(weights) {
		log.Fatalf("ids (%d), urls (%d) and weights (%d) must have equal length", len(ids), len(urls), len(weights))
	}

	servers := make([]*models.Server, len(ids))
	for i := range ids {
		servers[i] = models.NewServer(ids[i], urls[i], weights[i])
	}

	stats := newSelectionStats(ids)

	base, err := selectStrategy(*algorithm)
	if err != nil {
		log.Fatal(err)
	}

	strategy := &countingStrategy{inner: base, stats: stats}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go health.NewHealthChecker(servers).Start(ctx)
	go monitor.NewCPUCollector(servers).Start(ctx, 5*time.Second)

	proxyHandler := func(w http.ResponseWriter, r *http.Request) {
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

		start := time.Now()
		proxy.ServeHTTP(w, r)
		server.ObserveResponseTime(time.Since(start))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /bench/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, stats.snapshot())
	})
	mux.HandleFunc("/", proxyHandler)

	log.Printf("benchmark load balancer listening on %s (algorithm=%s)", *addr, *algorithm)

	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func selectStrategy(name string) (algorithms.Strategy, error) {
	switch name {
	case "round-robin":
		return &algorithms.RoundRobin{}, nil
	case "least-connections":
		return &algorithms.LeastConnections{}, nil
	case "weighted-round-robin":
		return &algorithms.WeightedRoundRobin{}, nil
	case "adaptive":
		return algorithms.NewAdaptive(algorithms.DefaultAdaptiveConfig()), nil
	default:
		return nil, fmt.Errorf("unknown algorithm %q", name)
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseWeights(value string) []int {
	parts := splitCSV(value)
	weights := make([]int, 0, len(parts))
	for _, part := range parts {
		weight, err := strconv.Atoi(part)
		if err != nil {
			weight = 0
		}
		weights = append(weights, weight)
	}
	return weights
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("encode response: %v", err)
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) {
	return len(p), nil
}
