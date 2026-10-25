package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/bench/scenario"
	"github.com/tejasmcodes/high-performance-request-system/bench/workload"
)

func main() {
	scenarioPath := flag.String("scenario", "", "path to a scenario JSON file")
	backendBin := flag.String("backend-bin", "bench/bin/bench-backend", "path to the benchmark backend binary")
	lbBin := flag.String("lb-bin", "bench/bin/bench-loadbalancer", "path to the benchmark load balancer binary")
	resultsDir := flag.String("results", "bench/results", "directory for generated results")
	durationOverride := flag.Int("duration", 0, "override load.duration_s (0 keeps scenario value)")
	warmupOverride := flag.Int("warmup", -1, "override load.warmup_s (-1 keeps scenario value)")
	concurrencyOverride := flag.Int("concurrency", 0, "override load.concurrency (0 keeps scenario value)")
	rateOverride := flag.Float64("rate", -1, "override load.rate_per_second (-1 keeps scenario value)")
	repeatsOverride := flag.Int("repeats", 0, "override repeats (0 keeps scenario value)")
	strategiesOverride := flag.String("strategies", "", "comma-separated strategy filter")
	cpuSetsFlag := flag.String("cpu-sets", "", "comma-separated taskset CPU lists per backend (empty disables pinning)")
	cooldown := flag.Duration("cooldown", 2*time.Second, "pause between runs")
	flag.Parse()

	if *scenarioPath == "" {
		flag.Usage()
		os.Exit(2)
	}

	sc, err := scenario.LoadScenario(*scenarioPath)
	if err != nil {
		log.Fatalf("load scenario: %v", err)
	}

	if *durationOverride > 0 {
		sc.Load.DurationS = *durationOverride
	}
	if *warmupOverride >= 0 {
		sc.Load.WarmupS = *warmupOverride
	}
	if *concurrencyOverride > 0 {
		sc.Load.Concurrency = *concurrencyOverride
	}
	if *rateOverride >= 0 {
		sc.Load.RatePerSecond = *rateOverride
	}
	if *repeatsOverride > 0 {
		sc.Repeats = *repeatsOverride
	}
	if *strategiesOverride != "" {
		sc.Strategies = splitCSV(*strategiesOverride)
	}

	if err := os.MkdirAll(*resultsDir, 0o755); err != nil {
		log.Fatalf("create results dir: %v", err)
	}

	cpuSets := splitCSV(*cpuSetsFlag)
	var results []*workload.Result

	for repeat := 1; repeat <= sc.Repeats; repeat++ {
		for _, strategy := range rotate(sc.Strategies, repeat-1) {
			fmt.Printf("running scenario=%s strategy=%s run=%d\n", sc.Name, strategy, repeat)

			result, err := runOnce(sc, strategy, repeat, *backendBin, *lbBin, *resultsDir, cpuSets)
			if err != nil {
				log.Fatalf("scenario=%s strategy=%s run=%d: %v", sc.Name, strategy, repeat, err)
			}

			resultPath := filepath.Join(*resultsDir, fmt.Sprintf("%s_%s_run%d.json", sc.Name, strategy, repeat))
			if err := result.WriteJSON(resultPath); err != nil {
				log.Fatalf("write result: %v", err)
			}

			results = append(results, result)
			fmt.Println(result.SummaryLine())

			time.Sleep(*cooldown)
		}
	}

	summary := &workload.Summary{
		Scenario:  sc.Name,
		CreatedAt: time.Now(),
		Results:   results,
	}
	summaryPath := filepath.Join(*resultsDir, sc.Name+"_summary.json")
	if err := workload.WriteSummary(summaryPath, summary); err != nil {
		log.Fatalf("write summary: %v", err)
	}

	printTable(results)
	fmt.Printf("summary written to %s\n", summaryPath)
}

func runOnce(sc *scenario.Scenario, strategy string, repeat int, backendBin, lbBin, resultsDir string, cpuSets []string) (*workload.Result, error) {
	logPath := filepath.Join(resultsDir, fmt.Sprintf("%s_%s_run%d.log", sc.Name, strategy, repeat))
	decisionLogPath := decisionLogPath(resultsDir, sc.Name, strategy, repeat)

	var procs []*process
	defer func() {
		for _, p := range procs {
			p.stop()
		}
	}()

	for i, backend := range sc.Backends {
		bin := backendBin
		args := []string{
			"-id", backend.ID,
			"-port", strconv.Itoa(backend.Port),
			"-latency", strconv.Itoa(backend.LatencyMS),
		}

		if i < len(cpuSets) && cpuSets[i] != "" {
			args = append([]string{"-c", cpuSets[i], backendBin}, args...)
			bin = "taskset"
		}

		p, err := startProcess(bin, args, logPath)
		if err != nil {
			return nil, fmt.Errorf("start backend %s: %w", backend.ID, err)
		}
		procs = append(procs, p)
	}

	for _, backend := range sc.Backends {
		healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", backend.Port)
		if err := waitHTTP(healthURL, 10*time.Second); err != nil {
			return nil, err
		}
	}

	lbArgs := loadBalancerArgs(sc, strategy, decisionLogPath)

	lbProc, err := startProcess(lbBin, lbArgs, logPath)
	if err != nil {
		return nil, fmt.Errorf("start load balancer: %w", err)
	}
	procs = append(procs, lbProc)

	if err := waitHTTP(lbStatsURL(sc.LBAddr), 10*time.Second); err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := workload.Config{
		URL:           lbRootURL(sc.LBAddr),
		Warmup:        time.Duration(sc.Load.WarmupS) * time.Second,
		Duration:      time.Duration(sc.Load.DurationS) * time.Second,
		Concurrency:   sc.Load.Concurrency,
		RatePerSecond: sc.Load.RatePerSecond,
		Scenario:      sc.Name,
		Strategy:      strategy,
		RunIndex:      repeat,
		Observe:       sc.URLs(),
		OnMeasurementStart: func(start time.Time) {
			go runPhases(runCtx, sc, start)
		},
	}

	result, err := workload.Run(runCtx, cfg)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func decisionLogPath(resultsDir, scenarioName, strategy string, run int) string {
	return filepath.Join(resultsDir, fmt.Sprintf("%s_%s_run%d_decisions.jsonl", scenarioName, strategy, run))
}

func loadBalancerArgs(sc *scenario.Scenario, strategy, decisionLogPath string) []string {
	return []string{
		"-algorithm", strategy,
		"-addr", sc.LBAddr,
		"-ids", strings.Join(sc.IDs(), ","),
		"-urls", strings.Join(sc.URLs(), ","),
		"-weights", scenario.JoinInts(weights(sc)),
		"-decision-log", decisionLogPath,
	}
}

func runPhases(ctx context.Context, sc *scenario.Scenario, start time.Time) {
	for _, phase := range sc.Phases {
		target := start.Add(time.Duration(phase.AtS) * time.Second)

		timer := time.NewTimer(time.Until(target))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		if phase.Action == "none" {
			continue
		}

		backendURL, ok := sc.BackendURL(phase.Backend)
		if !ok {
			continue
		}

		switch phase.Action {
		case "set_latency":
			post(fmt.Sprintf("%s/bench/latency?ms=%d", backendURL, phase.LatencyMS))
		case "set_cpu":
			post(fmt.Sprintf("%s/bench/cpu?workers=%d", backendURL, phase.CPUWorkers))
		}
	}
}

func post(url string) {
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)
}

func weights(sc *scenario.Scenario) []int {
	values := make([]int, len(sc.Backends))
	for i, backend := range sc.Backends {
		values[i] = backend.Weight
	}
	return values
}

func rotate(values []string, by int) []string {
	if len(values) == 0 {
		return values
	}

	out := make([]string, len(values))
	for i := range values {
		out[i] = values[(i+by)%len(values)]
	}
	return out
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

func lbRootURL(addr string) string {
	return "http://" + normalizeAddr(addr) + "/"
}

func lbStatsURL(addr string) string {
	return "http://" + normalizeAddr(addr) + "/bench/stats"
}

func normalizeAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}

func waitHTTP(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for %s", url)
}

func printTable(results []*workload.Result) {
	fmt.Printf("%-22s %-22s %4s %12s %11s %10s %10s %10s\n",
		"scenario", "strategy", "run", "requests", "error_rate", "mean_ms", "p95_ms", "p99_ms")

	for _, result := range results {
		fmt.Printf("%-22s %-22s %4d %12d %11.4f %10.2f %10.2f %10.2f\n",
			result.Scenario,
			result.Strategy,
			result.Run,
			result.TotalRequests,
			result.ErrorRate,
			result.Latency.MeanMS,
			result.Latency.P95MS,
			result.Latency.P99MS,
		)
	}
}

type process struct {
	cmd     *exec.Cmd
	logFile *os.File
}

func startProcess(bin string, args []string, logPath string) (*process, error) {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, err
	}

	return &process{cmd: cmd, logFile: logFile}, nil
}

func (p *process) stop() {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	_ = p.cmd.Wait()
	_ = p.logFile.Close()
}
