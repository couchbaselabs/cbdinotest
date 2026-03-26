//go:build windows

package metrics

import (
	"sync"
	"time"
)

// SelfMetricsCollector is a stub for Windows that always returns zero values.
type SelfMetricsCollector struct {
	mu              sync.Mutex
	lastTime        time.Time
	lastCPUTimeUsec int64
}

// NewSelfMetricsCollector creates a collector (stub on Windows).
func NewSelfMetricsCollector() *SelfMetricsCollector {
	return &SelfMetricsCollector{}
}

// Sample returns zero CPU percentage and zero RSS bytes on Windows.
func (c *SelfMetricsCollector) Sample() (cpuPercent float64, rss int64) {
	return 0, 0
}
