package metrics

import (
	"testing"
	"time"
)

func TestResponseTimeFirstObservation(t *testing.T) {
	metric := NewResponseTime(0.2)

	metric.Observe(100 * time.Millisecond)

	if got := metric.Value(); got != 100 {
		t.Fatalf("expected 100ms, got %.2f ms",got)
	}
}

func TestResponseTimeEMA(t *testing.T){
	metric := NewResponseTime(0.2)
	metric.Observe(100 * time.Millisecond)
	metric.Observe(200 * time.Millisecond)

	// EMA = 0.2*200 + 0.8*100
	expected := 120.0

	if got := metric.Value(); got !=  expected {
		t.Fatalf("expected %.2f ms, got %.2f ms",expected,got)
	}
}

func TestResponseTimeCount(t *testing.T) {
	metric := NewResponseTime(0.2)

	metric.Observe(100 * time.Millisecond)
	metric.Observe(200 * time.Millisecond)
	metric.Observe(300 * time.Millisecond)

	if got := metric.Count(); got != 3 {
		t.Fatalf("expected count 3, got %d", got)
	}
}

func TestResponseTimeInvalidAlpha(t *testing.T) {
	metric := NewResponseTime(2)

	metric.Observe(100 * time.Millisecond)
	metric.Observe(200 * time.Millisecond)

	// Invalid alpha should fall back to 0.2.
	// EMA = 0.2*200 + 0.8*100 = 120
	expected := 120.0

	if got := metric.Value(); got != expected {
		t.Fatalf("expected %.2f ms, got %.2f ms", expected, got)
	}
}