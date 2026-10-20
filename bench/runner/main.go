package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/bench/workload"
)

func main() {
	url := flag.String("url", "http://127.0.0.1:18080/", "load balancer URL")
	warmup := flag.Int("warmup", 5, "warmup duration in seconds")
	duration := flag.Int("duration", 30, "measurement duration in seconds")
	concurrency := flag.Int("concurrency", 32, "concurrent clients")
	rate := flag.Float64("rate", 0, "target requests per second (0 = closed loop)")
	timeout := flag.Duration("timeout", 5*time.Second, "per-request timeout")
	scenario := flag.String("scenario", "baseline", "scenario name")
	strategy := flag.String("strategy", "adaptive", "strategy name")
	runIndex := flag.Int("run", 1, "run index")
	observe := flag.String("observe", "", "comma-separated backend base URLs for CPU polling")
	out := flag.String("out", "", "optional JSON result output path")
	flag.Parse()

	cfg := workload.Config{
		URL:           *url,
		Warmup:        time.Duration(*warmup) * time.Second,
		Duration:      time.Duration(*duration) * time.Second,
		Concurrency:   *concurrency,
		RatePerSecond: *rate,
		Timeout:       *timeout,
		Scenario:      *scenario,
		Strategy:      *strategy,
		RunIndex:      *runIndex,
		Observe:       splitURLs(*observe),
	}

	result, err := workload.Run(context.Background(), cfg)
	if err != nil {
		log.Fatalf("workload failed: %v", err)
	}

	fmt.Println(result.SummaryLine())

	if *out != "" {
		if err := result.WriteJSON(*out); err != nil {
			log.Fatalf("write result: %v", err)
		}
	}
}

func splitURLs(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	urls := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			urls = append(urls, strings.TrimSuffix(trimmed, "/"))
		}
	}

	return urls
}
