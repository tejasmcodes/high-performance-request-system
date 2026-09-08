package metrics

import (
	"sync"
	"time"
)

type ResponseTime struct {
	mu    sync.RWMutex
	emaMs float64
	alpha float64
	count int64
}

func NewResponseTime(alpha float64) *ResponseTime {
	if alpha <= 0 || alpha > 1 {
		alpha = 0.2
	}

	return &ResponseTime{
		alpha: alpha,
	}
}

func (m *ResponseTime) Observe(duration time.Duration) {
	ms := float64(duration.Microseconds()) / 1000.0

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.count == 0 {
		m.emaMs = ms
	} else {
		m.emaMs = m.alpha*ms + (1-m.alpha)*m.emaMs
	}
	m.count++
}

func (m *ResponseTime) Value() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.emaMs
}

func (m *ResponseTime) Count() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.count
}
