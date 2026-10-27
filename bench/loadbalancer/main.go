package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/algorithms"
	"github.com/tejasmcodes/high-performance-request-system/internal/benchmark"
	"github.com/tejasmcodes/high-performance-request-system/internal/decisionlog"
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

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}

	return w.ResponseWriter.Write(data)
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *statusResponseWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}

	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}

	return hijacker.Hijack()
}

func (w *statusResponseWriter) Push(target string, options *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}

	return pusher.Push(target, options)
}

func (w *statusResponseWriter) ReadFrom(reader io.Reader) (int64, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}

	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(reader)
	}

	return io.Copy(w.ResponseWriter, reader)
}

func (w *statusResponseWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}

	return w.status
}

type benchmarkProxyHandler struct {
	strategy     algorithms.Strategy
	servers      []*models.Server
	recorder     *decisionlog.Recorder
	strategyName string
}

func (h *benchmarkProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	responseWriter := w
	var statusWriter *statusResponseWriter

	var (
		selectionTime time.Time
		candidates    []decisionlog.CandidateSnapshot
	)
	if h.recorder != nil {
		statusWriter = &statusResponseWriter{ResponseWriter: w}
		responseWriter = statusWriter
		selectionTime = time.Now()
		candidates = decisionlog.SnapshotServers(h.servers, selectionTime)
	}

	server := h.strategy.NextServer(h.servers)
	selectedBackend := ""
	if server != nil {
		selectedBackend = server.ID
	}

	recordDecision := func(requestErr error, proxyError bool) {
		if h.recorder == nil {
			return
		}

		completionTime := time.Now()
		outcome := decisionlog.Outcome{
			CompletionTime:  completionTime,
			LatencyMs:       float64(completionTime.Sub(selectionTime)) / float64(time.Millisecond),
			HTTPStatus:      statusWriter.Status(),
			Success:         requestErr == nil && statusWriter.Status() >= http.StatusOK && statusWriter.Status() < http.StatusBadRequest,
			Timeout:         errors.Is(r.Context().Err(), context.DeadlineExceeded),
			ContextCanceled: errors.Is(r.Context().Err(), context.Canceled),
			ProxyError:      proxyError,
		}
		if requestErr != nil {
			outcome.Error = requestErr.Error()
		}

		record := decisionlog.Record{
			SchemaVersion:   decisionlog.SchemaVersion,
			DecisionID:      h.recorder.NextDecisionID(),
			SelectionTime:   selectionTime,
			Strategy:        h.strategyName,
			SelectedBackend: selectedBackend,
			Candidates:      candidates,
			Outcome:         outcome,
		}
		if err := h.recorder.Record(record); err != nil {
			log.Printf("write decision log: %v", err)
		}
	}

	if server == nil {
		requestErr := errors.New("no healthy backend available")
		http.Error(responseWriter, requestErr.Error(), http.StatusServiceUnavailable)
		recordDecision(requestErr, false)
		return
	}

	server.IncrementConnections()
	defer server.DecrementConnections()

	var proxyErr error
	proxy, err := benchmark.NewReverseProxy(server, func(w http.ResponseWriter, r *http.Request, err error) {
		proxyErr = err
		log.Printf("proxy error: %v", err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
	})
	if err != nil {
		http.Error(responseWriter, "invalid backend URL", http.StatusInternalServerError)
		recordDecision(err, false)
		return
	}

	start := time.Now()
	proxy.ServeHTTP(responseWriter, r)
	server.ObserveResponseTime(time.Since(start))
	recordDecision(proxyErr, proxyErr != nil)
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
	decisionLogPath := flag.String("decision-log", "", "decision log JSONL output path (empty disables logging)")
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

	var recorder *decisionlog.Recorder
	if *decisionLogPath != "" {
		recorder, err = decisionlog.OpenRecorder(*decisionLogPath)
		if err != nil {
			log.Fatalf("open decision log: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go health.NewHealthChecker(servers).Start(ctx)
	go monitor.NewCPUCollector(servers).Start(ctx, 5*time.Second)

	proxyHandler := &benchmarkProxyHandler{
		strategy:     strategy,
		servers:      servers,
		recorder:     recorder,
		strategyName: *algorithm,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /bench/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, stats.snapshot())
	})
	mux.Handle("/", proxyHandler)

	log.Printf("benchmark load balancer listening on %s (algorithm=%s)", *addr, *algorithm)

	listenErr := http.ListenAndServe(*addr, mux)
	if recorder != nil {
		if err := recorder.Close(); err != nil {
			log.Printf("close decision log: %v", err)
		}
	}
	if listenErr != nil {
		log.Fatal(listenErr)
	}
}

func selectStrategy(name string) (algorithms.Strategy, error) {
	return benchmark.NewStrategy(name)
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
