package metrics

import (
	"sync"
	"time"
)

type Snapshot struct {
	StartedAt        time.Time `json:"started_at"`
	TotalRequests    int64     `json:"total_requests"`
	Successful       int64     `json:"successful"`
	Failed           int64     `json:"failed"`
	AverageLatencyMS float64   `json:"average_latency_ms"`
}

type Collector struct {
	mu             sync.RWMutex
	startedAt      time.Time
	totalRequests  int64
	successful     int64
	failed         int64
	totalLatencyMS int64
}

func New() *Collector {
	return &Collector{
		startedAt: time.Now().UTC(),
	}
}

func (c *Collector) Record(duration time.Duration, success bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.totalRequests++
	c.totalLatencyMS += duration.Milliseconds()
	if success {
		c.successful++
		return
	}
	c.failed++
}

func (c *Collector) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	average := 0.0
	if c.totalRequests > 0 {
		average = float64(c.totalLatencyMS) / float64(c.totalRequests)
	}

	return Snapshot{
		StartedAt:        c.startedAt,
		TotalRequests:    c.totalRequests,
		Successful:       c.successful,
		Failed:           c.failed,
		AverageLatencyMS: average,
	}
}
