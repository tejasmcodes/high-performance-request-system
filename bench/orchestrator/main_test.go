package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tejasmcodes/high-performance-request-system/bench/scenario"
)

func TestDecisionLogPath(t *testing.T) {
	got := decisionLogPath("bench/results", "degradation", "adaptive", 2)
	want := filepath.Join("bench/results", "degradation_adaptive_run2_decisions.jsonl")

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestLoadBalancerArgsIncludesDecisionLogPath(t *testing.T) {
	sc := &scenario.Scenario{
		LBAddr: ":18080",
		Backends: []scenario.Backend{
			{ID: "backend-1", Port: 18081, Weight: 3},
			{ID: "backend-2", Port: 18082, Weight: 1},
		},
	}
	decisionLog := "bench/results/degradation_adaptive_run2_decisions.jsonl"

	got := loadBalancerArgs(sc, "adaptive", decisionLog)
	want := []string{
		"-algorithm", "adaptive",
		"-addr", ":18080",
		"-ids", "backend-1,backend-2",
		"-urls", "http://127.0.0.1:18081,http://127.0.0.1:18082",
		"-weights", "3,1",
		"-decision-log", decisionLog,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected args %q, got %q", want, got)
	}
}
