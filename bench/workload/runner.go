package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"
)

type Config struct {
	URL                string
	Warmup             time.Duration
	Duration           time.Duration
	Concurrency        int
	RatePerSecond      float64
	Timeout            time.Duration
	Scenario           string
	Strategy           string
	RunIndex           int
	Observe            []string
	OnMeasurementStart func(start time.Time)
}

type windowAccumulator struct {
	requests uint64
	errors   uint64
	hist     *Histogram
	backends map[string]uint64
}

type accumulator struct {
	requests uint64
	errors   uint64
	hist     *Histogram
	backends map[string]uint64
	windows  []*windowAccumulator
}

func newAccumulator(windowCount int) *accumulator {
	windows := make([]*windowAccumulator, windowCount)
	for i := range windows {
		windows[i] = &windowAccumulator{
			hist:     NewHistogram(),
			backends: make(map[string]uint64),
		}
	}

	return &accumulator{
		hist:     NewHistogram(),
		backends: make(map[string]uint64),
		windows:  windows,
	}
}

func (a *accumulator) record(latency, offset time.Duration, backend string, failed bool) {
	a.requests++
	a.hist.Record(latency)

	index := int(offset / time.Second)
	if index < 0 {
		index = 0
	}
	if index >= len(a.windows) {
		index = len(a.windows) - 1
	}

	window := a.windows[index]
	window.requests++
	window.hist.Record(latency)

	if backend != "" {
		a.backends[backend]++
		window.backends[backend]++
	}

	if failed {
		a.errors++
		window.errors++
	}
}

func (a *accumulator) mergeInto(target *accumulator) {
	target.requests += a.requests
	target.errors += a.errors
	target.hist.Merge(a.hist)

	for backend, count := range a.backends {
		target.backends[backend] += count
	}

	for i, window := range a.windows {
		target.windows[i].requests += window.requests
		target.windows[i].errors += window.errors
		target.windows[i].hist.Merge(window.hist)
		for backend, count := range window.backends {
			target.windows[i].backends[backend] += count
		}
	}
}

func Run(ctx context.Context, cfg Config) (*Result, error) {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}

	windowCount := int(math.Ceil(cfg.Duration.Seconds()))
	if windowCount <= 0 {
		windowCount = 1
	}

	client := newHTTPClient(cfg)
	defer client.CloseIdleConnections()

	if cfg.Warmup > 0 {
		warmup := makeAccumulators(cfg.Concurrency, windowCount)
		runPhase(ctx, cfg, client, cfg.Warmup, time.Now(), warmup)
	}

	accs := makeAccumulators(cfg.Concurrency, windowCount)

	started := time.Now()
	if cfg.OnMeasurementStart != nil {
		cfg.OnMeasurementStart(started)
	}

	pollerCtx, stopPoller := context.WithCancel(ctx)
	poller := newCPUPoller(cfg.Observe)
	poller.start(pollerCtx)

	runPhase(ctx, cfg, client, cfg.Duration, started, accs)

	stopPoller()

	finished := time.Now()

	merged := newAccumulator(windowCount)
	for _, acc := range accs {
		acc.mergeInto(merged)
	}

	return buildResult(cfg, merged, poller.snapshot(), started, finished), nil
}

func newHTTPClient(cfg Config) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        cfg.Concurrency * 2,
		MaxIdleConnsPerHost: cfg.Concurrency * 2,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
	}
}

func makeAccumulators(concurrency, windowCount int) []*accumulator {
	accs := make([]*accumulator, concurrency)
	for i := range accs {
		accs[i] = newAccumulator(windowCount)
	}
	return accs
}

func runPhase(ctx context.Context, cfg Config, client *http.Client, duration time.Duration, measureStart time.Time, accs []*accumulator) {
	if duration <= 0 {
		return
	}

	deadline := time.Now().Add(duration)

	if cfg.RatePerSecond > 0 {
		runOpenLoop(ctx, cfg, client, deadline, measureStart, accs)
		return
	}

	runClosedLoop(ctx, cfg, client, deadline, measureStart, accs)
}

func runClosedLoop(ctx context.Context, cfg Config, client *http.Client, deadline, measureStart time.Time, accs []*accumulator) {
	var wg sync.WaitGroup

	for _, acc := range accs {
		wg.Add(1)

		go func(acc *accumulator) {
			defer wg.Done()

			for ctx.Err() == nil && time.Now().Before(deadline) {
				start := time.Now()
				backend, ok, err := doRequest(ctx, cfg, client)
				latency := time.Since(start)
				acc.record(latency, time.Since(measureStart), backend, !ok || err != nil)
			}
		}(acc)
	}

	wg.Wait()
}

func runOpenLoop(ctx context.Context, cfg Config, client *http.Client, deadline, measureStart time.Time, accs []*accumulator) {
	interval := time.Duration(float64(time.Second) / cfg.RatePerSecond)
	if interval <= 0 {
		interval = time.Nanosecond
	}

	jobs := make(chan struct{})
	var wg sync.WaitGroup

	for _, acc := range accs {
		wg.Add(1)

		go func(acc *accumulator) {
			defer wg.Done()

			for range jobs {
				start := time.Now()
				backend, ok, err := doRequest(ctx, cfg, client)
				latency := time.Since(start)
				acc.record(latency, time.Since(measureStart), backend, !ok || err != nil)
			}
		}(acc)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for ctx.Err() == nil && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
		case <-ticker.C:
			select {
			case jobs <- struct{}{}:
			case <-ctx.Done():
			}
		}
	}

	close(jobs)
	wg.Wait()
}

func doRequest(ctx context.Context, cfg Config, client *http.Client) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		return "", false, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)

	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	return resp.Header.Get("X-Backend-ID"), ok, nil
}

type cpuSample struct {
	at      time.Time
	backend string
	value   float64
}

type cpuPoller struct {
	backends []string

	mu      sync.Mutex
	samples []cpuSample
	client  *http.Client
}

func newCPUPoller(backends []string) *cpuPoller {
	return &cpuPoller{
		backends: backends,
		client:   &http.Client{Timeout: 2 * time.Second},
	}
}

func (p *cpuPoller) start(ctx context.Context) {
	if len(p.backends) == 0 {
		return
	}

	go func() {
		p.poll(ctx)

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.poll(ctx)
			}
		}
	}()
}

func (p *cpuPoller) poll(ctx context.Context) {
	at := time.Now()

	for _, backend := range p.backends {
		id, value, err := p.fetchCPU(ctx, backend)
		if err != nil {
			continue
		}

		if id == "" {
			id = backend
		}

		p.mu.Lock()
		p.samples = append(p.samples, cpuSample{at: at, backend: id, value: value})
		p.mu.Unlock()
	}
}

func (p *cpuPoller) fetchCPU(ctx context.Context, backend string) (string, float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, backend+"/metrics/cpu", nil)
	if err != nil {
		return "", 0, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var payload struct {
		ID             string  `json:"id"`
		CPUUtilization float64 `json:"cpu_utilization"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", 0, err
	}

	return payload.ID, payload.CPUUtilization, nil
}

func (p *cpuPoller) snapshot() []cpuSample {
	p.mu.Lock()
	defer p.mu.Unlock()

	samples := make([]cpuSample, len(p.samples))
	copy(samples, p.samples)
	return samples
}

func buildResult(cfg Config, acc *accumulator, samples []cpuSample, started, finished time.Time) *Result {
	result := &Result{
		Scenario:        cfg.Scenario,
		Strategy:        cfg.Strategy,
		Run:             cfg.RunIndex,
		URL:             cfg.URL,
		Concurrency:     cfg.Concurrency,
		RatePerSecond:   cfg.RatePerSecond,
		WarmupSeconds:   cfg.Warmup.Seconds(),
		DurationSeconds: cfg.Duration.Seconds(),
		StartedAt:       started,
		FinishedAt:      finished,
		TotalRequests:   acc.requests,
		TotalErrors:     acc.errors,
		Latency:         acc.hist.Snapshot(),
		Backends:        acc.backends,
	}

	if acc.requests > 0 {
		result.ErrorRate = float64(acc.errors) / float64(acc.requests)
	}

	result.Windows = make([]WindowReport, len(acc.windows))
	for i, window := range acc.windows {
		result.Windows[i] = WindowReport{
			Second:   i,
			Requests: window.requests,
			Errors:   window.errors,
			Latency:  window.hist.Snapshot(),
			Backends: window.backends,
			CPU:      cpuAtWindow(samples, started, i),
		}
	}

	return result
}

func cpuAtWindow(samples []cpuSample, started time.Time, second int) map[string]float64 {
	windowEnd := started.Add(time.Duration(second+1) * time.Second)

	latest := make(map[string]cpuSample)
	for _, sample := range samples {
		if sample.at.After(windowEnd) {
			continue
		}
		current, ok := latest[sample.backend]
		if !ok || sample.at.After(current.at) {
			latest[sample.backend] = sample
		}
	}

	if len(latest) == 0 {
		return nil
	}

	values := make(map[string]float64, len(latest))
	for backend, sample := range latest {
		values[backend] = sample.value
	}

	return values
}

func (r *Result) SummaryLine() string {
	return fmt.Sprintf(
		"scenario=%s strategy=%s run=%d requests=%d error_rate=%.4f mean=%.2fms p50=%.2fms p95=%.2fms p99=%.2fms",
		r.Scenario,
		r.Strategy,
		r.Run,
		r.TotalRequests,
		r.ErrorRate,
		r.Latency.MeanMS,
		r.Latency.P50MS,
		r.Latency.P95MS,
		r.Latency.P99MS,
	)
}
