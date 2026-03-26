//go:build !windows

package metrics

import (
	"runtime"
	"sync"
	"syscall"
	"time"
)

// cpuTimeUsec returns the total user + system CPU time in microseconds.
func cpuTimeUsec(ru *syscall.Rusage) int64 {
	return int64(ru.Utime.Sec)*1e6 +
		int64(ru.Utime.Usec) +
		int64(ru.Stime.Sec)*1e6 +
		int64(ru.Stime.Usec)
}

// rssBytes returns the resident set size in bytes.
// On Linux ru_maxrss is in kilobytes; on macOS it is already in bytes.
func rssBytes(ru *syscall.Rusage) int64 {
	if runtime.GOOS == "linux" {
		return ru.Maxrss * 1024
	}
	return ru.Maxrss
}

// SelfMetricsCollector samples CPU and memory usage for the current process
// using syscall.Getrusage, matching the approach used by the test-service
// worker metrics endpoint.
type SelfMetricsCollector struct {
	mu              sync.Mutex
	lastTime        time.Time
	lastCPUTimeUsec int64
}

// NewSelfMetricsCollector creates a collector with an initial CPU baseline.
func NewSelfMetricsCollector() *SelfMetricsCollector {
	c := &SelfMetricsCollector{}

	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err == nil {
		c.lastTime = time.Now()
		c.lastCPUTimeUsec = cpuTimeUsec(&rusage)
	}

	return c
}

// Sample returns the current CPU percentage and RSS bytes.
// CPU percentage is computed as the delta of CPU time over wall-clock time
// since the previous call.
func (c *SelfMetricsCollector) Sample() (cpuPercent float64, rss int64) {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		return 0, 0
	}

	now := time.Now()
	currentCPU := cpuTimeUsec(&rusage)

	c.mu.Lock()
	if !c.lastTime.IsZero() {
		wallElapsed := now.Sub(c.lastTime).Microseconds()
		if wallElapsed > 0 {
			cpuDelta := currentCPU - c.lastCPUTimeUsec
			cpuPercent = float64(cpuDelta) / float64(wallElapsed) * 100.0
		}
	}
	c.lastTime = now
	c.lastCPUTimeUsec = currentCPU
	c.mu.Unlock()

	return cpuPercent, rssBytes(&rusage)
}
