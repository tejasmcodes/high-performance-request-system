package benchmark

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/bench/workload"
	"github.com/tejasmcodes/high-performance-request-system/internal/algorithms"
	"github.com/tejasmcodes/high-performance-request-system/internal/health"
	"github.com/tejasmcodes/high-performance-request-system/internal/models"
	"github.com/tejasmcodes/high-performance-request-system/internal/monitor"
)

type ServerConfig struct {
	ID     string
	URL    string
	Weight int
}

type Config struct {
	Algorithm      string
	Scenario       string
	Servers        []ServerConfig
	Duration       time.Duration
	Warmup         time.Duration
	Concurrency    int
	RatePerSecond  float64
	RequestTimeout time.Duration
}

type Service struct{}

func (Service) Run(ctx context.Context, cfg Config) (*workload.Result, error) {
	return Run(ctx, cfg)
}

func Run(ctx context.Context, cfg Config) (*workload.Result, error) {
	strategy, err := NewStrategy(cfg.Algorithm)
	if err != nil {
		return nil, err
	}

	servers := make([]*models.Server, len(cfg.Servers))
	for i, server := range cfg.Servers {
		servers[i] = models.NewServer(server.ID, strings.TrimRight(server.URL, "/"), server.Weight)
	}

	checker := health.NewHealthChecker(servers)
	checker.CheckAll()
	for _, server := range servers {
		if !server.IsHealthy() {
			return nil, fmt.Errorf("backend %q failed its health check", server.ID)
		}
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go checker.Start(runCtx)
	go monitor.NewCPUCollector(servers).Start(runCtx, 5*time.Second)

	lb := httptest.NewServer(proxyHandler{strategy: strategy, servers: servers})
	defer lb.Close()

	return workload.Run(runCtx, workload.Config{
		URL:           lb.URL,
		Warmup:        cfg.Warmup,
		Duration:      cfg.Duration,
		Concurrency:   cfg.Concurrency,
		RatePerSecond: cfg.RatePerSecond,
		Timeout:       cfg.RequestTimeout,
		Scenario:      cfg.Scenario,
		Strategy:      cfg.Algorithm,
		Observe:       serverURLs(cfg.Servers),
	})
}

func NewStrategy(name string) (algorithms.Strategy, error) {
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

func NewReverseProxy(server *models.Server, errorHandler func(http.ResponseWriter, *http.Request, error)) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(server.URL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(response *http.Response) error {
		if response.Header.Get("X-Backend-ID") == "" {
			response.Header.Set("X-Backend-ID", server.ID)
		}
		return nil
	}
	if errorHandler != nil {
		proxy.ErrorHandler = errorHandler
	}
	return proxy, nil
}

func serverURLs(servers []ServerConfig) []string {
	urls := make([]string, len(servers))
	for i, server := range servers {
		urls[i] = strings.TrimRight(server.URL, "/")
	}
	return urls
}

type proxyHandler struct {
	strategy algorithms.Strategy
	servers  []*models.Server
}

func (h proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	server := h.strategy.NextServer(h.servers)
	if server == nil {
		http.Error(w, "no healthy backend available", http.StatusServiceUnavailable)
		return
	}

	proxy, err := NewReverseProxy(server, func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
	})
	if err != nil {
		http.Error(w, "invalid backend URL", http.StatusBadGateway)
		return
	}

	server.IncrementConnections()
	defer server.DecrementConnections()

	start := time.Now()
	proxy.ServeHTTP(w, r)
	server.ObserveResponseTime(time.Since(start))
}
