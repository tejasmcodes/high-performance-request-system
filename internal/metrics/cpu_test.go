package metrics

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestReadProcessCPUTime(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process CPU sampling requires /proc/self/schedstat")
	}

	before, err := readProcessCPUTime()
	if err != nil {
		t.Fatalf("read process CPU time: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	after, err := readProcessCPUTime()
	if err != nil {
		t.Fatalf("read process CPU time: %v", err)
	}

	if after < before {
		t.Fatalf("process CPU time went backwards: %d -> %d", before, after)
	}
}

func TestProcessCPUUtilization(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process CPU sampling requires /proc/self/schedstat")
	}

	value, err := ProcessCPUUtilization()
	if err != nil {
		t.Fatalf("process CPU utilization: %v", err)
	}

	if value < 0 || value > 100 {
		t.Fatalf("expected utilization within 0-100, got %f", value)
	}
}

func TestCPUSamplerCachesValue(t *testing.T) {
	sampler := NewCPUSampler()
	sampler.sample = func() (float64, error) {
		return 42.5, nil
	}

	sampler.Sample()

	if got := sampler.Value(); got != 42.5 {
		t.Fatalf("expected 42.5, got %f", got)
	}
}

func TestCPUSamplerKeepsValueOnError(t *testing.T) {
	sampler := NewCPUSampler()
	sampler.sample = func() (float64, error) {
		return 10, nil
	}
	sampler.Sample()

	sampler.sample = func() (float64, error) {
		return 0, context.DeadlineExceeded
	}
	sampler.Sample()

	if got := sampler.Value(); got != 10 {
		t.Fatalf("expected previous value 10, got %f", got)
	}
}

func TestCPUSamplerRunStopsOnContextCancel(t *testing.T) {
	sampler := NewCPUSampler()
	sampler.sample = func() (float64, error) {
		return 1, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		sampler.Run(ctx, 10*time.Millisecond)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sampler did not stop after context cancellation")
	}
}

func TestCPUSamplerConcurrentAccess(t *testing.T) {
	sampler := NewCPUSampler()
	sampler.sample = func() (float64, error) {
		return 5, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sampler.Run(ctx, time.Millisecond)

	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				_ = sampler.Value()
			}
		}()
	}

	for i := 0; i < 8; i++ {
		<-done
	}
}
