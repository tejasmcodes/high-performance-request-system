package workload

import (
	"math"
	"time"
)

const (
	histogramBucketWidth = 100 * time.Microsecond
	histogramMaxDuration = 10 * time.Second
	histogramBucketCount = int(histogramMaxDuration/histogramBucketWidth) + 1
)

type HistogramSnapshot struct {
	Count  uint64  `json:"count"`
	MeanMS float64 `json:"mean_ms"`
	P50MS  float64 `json:"p50_ms"`
	P95MS  float64 `json:"p95_ms"`
	P99MS  float64 `json:"p99_ms"`
	MinMS  float64 `json:"min_ms"`
	MaxMS  float64 `json:"max_ms"`
}

type Histogram struct {
	buckets []uint64
	count   uint64
	sum     time.Duration
	min     time.Duration
	max     time.Duration
}

func NewHistogram() *Histogram {
	return &Histogram{
		buckets: make([]uint64, histogramBucketCount),
	}
}

func (h *Histogram) Record(duration time.Duration) {
	if duration < 0 {
		duration = 0
	}

	index := int(duration / histogramBucketWidth)
	if index >= histogramBucketCount {
		index = histogramBucketCount - 1
	}

	h.buckets[index]++
	h.count++
	h.sum += duration

	if h.count == 1 || duration < h.min {
		h.min = duration
	}
	if duration > h.max {
		h.max = duration
	}
}

func (h *Histogram) Merge(other *Histogram) {
	if other == nil {
		return
	}

	for i, count := range other.buckets {
		h.buckets[i] += count
	}

	h.count += other.count
	h.sum += other.sum

	if other.count > 0 {
		if h.count == other.count || other.min < h.min {
			h.min = other.min
		}
		if other.max > h.max {
			h.max = other.max
		}
	}
}

func (h *Histogram) Snapshot() HistogramSnapshot {
	return HistogramSnapshot{
		Count:  h.count,
		MeanMS: milliseconds(h.Mean()),
		P50MS:  milliseconds(h.Percentile(0.50)),
		P95MS:  milliseconds(h.Percentile(0.95)),
		P99MS:  milliseconds(h.Percentile(0.99)),
		MinMS:  milliseconds(h.min),
		MaxMS:  milliseconds(h.max),
	}
}

func (h *Histogram) Mean() time.Duration {
	if h.count == 0 {
		return 0
	}

	return h.sum / time.Duration(h.count)
}

func (h *Histogram) Percentile(q float64) time.Duration {
	if h.count == 0 {
		return 0
	}

	if q <= 0 {
		return h.min
	}
	if q >= 1 {
		return h.max
	}

	target := uint64(math.Ceil(q * float64(h.count)))
	if target == 0 {
		target = 1
	}

	var cumulative uint64
	for i, count := range h.buckets {
		cumulative += count
		if cumulative >= target {
			upper := time.Duration(i+1) * histogramBucketWidth
			if upper > h.max {
				return h.max
			}
			return upper
		}
	}

	return h.max
}

func milliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
