package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/metrics"
)

var burnSink uint64

type burner struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	active int
}

func (b *burner) Set(workers int) {
	if workers < 0 {
		workers = 0
	}
	if workers > 256 {
		workers = 256
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}

	b.active = workers
	if workers == 0 {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel

	for i := 0; i < workers; i++ {
		go burn(ctx)
	}
}

func (b *burner) Active() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.active
}

func burn(ctx context.Context) {
	x := uint64(0x9e3779b97f4a7c15)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		for i := 0; i < 4096; i++ {
			x ^= x << 13
			x ^= x >> 7
			x ^= x << 17
		}
		atomic.AddUint64(&burnSink, x)
	}
}

type backend struct {
	id      string
	latency int64
	burner  *burner

	requests uint64
	errors   uint64
}

func (b *backend) GetLatency() time.Duration {
	return time.Duration(atomic.LoadInt64(&b.latency)) * time.Millisecond
}

func (b *backend) SetLatencyMilliseconds(ms int64) {
	if ms < 0 {
		ms = 0
	}
	atomic.StoreInt64(&b.latency, ms)
}

func main() {
	id := flag.String("id", "backend-1", "backend identifier")
	port := flag.Int("port", 18081, "listening port")
	latency := flag.Int64("latency", 0, "per-request latency in milliseconds")
	flag.Parse()

	b := &backend{
		id:     *id,
		burner: &burner{},
	}
	b.SetLatencyMilliseconds(*latency)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sampler := metrics.NewCPUSampler()
	go sampler.Run(ctx, metrics.DefaultCPUSampleInterval)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if d := b.GetLatency(); d > 0 {
			time.Sleep(d)
		}

		atomic.AddUint64(&b.requests, 1)

		w.Header().Set("X-Backend-ID", b.id)
		fmt.Fprintf(w, "Hello from %s\n", b.id)
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})

	mux.HandleFunc("GET /metrics/cpu", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id":              b.id,
			"cpu_utilization": sampler.Value(),
		})
	})

	mux.HandleFunc("GET /bench/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id":          b.id,
			"requests":    atomic.LoadUint64(&b.requests),
			"errors":      atomic.LoadUint64(&b.errors),
			"latency_ms":  atomic.LoadInt64(&b.latency),
			"cpu_workers": b.burner.Active(),
			"num_cpu":     runtime.NumCPU(),
			"cpu":         sampler.Value(),
		})
	})

	mux.HandleFunc("POST /bench/latency", func(w http.ResponseWriter, r *http.Request) {
		ms, err := strconv.ParseInt(r.URL.Query().Get("ms"), 10, 64)
		if err != nil {
			http.Error(w, "invalid ms", http.StatusBadRequest)
			return
		}

		b.SetLatencyMilliseconds(ms)
		writeJSON(w, map[string]any{"id": b.id, "latency_ms": ms})
	})

	mux.HandleFunc("POST /bench/cpu", func(w http.ResponseWriter, r *http.Request) {
		workers, err := strconv.Atoi(r.URL.Query().Get("workers"))
		if err != nil {
			http.Error(w, "invalid workers", http.StatusBadRequest)
			return
		}

		b.burner.Set(workers)
		writeJSON(w, map[string]any{"id": b.id, "cpu_workers": b.burner.Active()})
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("%s benchmark backend listening on %s (latency=%dms)", b.id, addr, atomic.LoadInt64(&b.latency))

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		fmt.Fprintf(os.Stderr, "encode response: %v\n", err)
	}
}
