package metrics

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/couchbaselabs/cbdinotest/eventdb"
)

// ConsoleReporter periodically prints a summary of recent events to stdout.
type ConsoleReporter struct {
	eventLog *eventdb.EventLog
	interval time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup

	// Track where we left off in the event log so each tick only
	// processes new events since the last report.
	lastIdx int
}

// NewConsoleReporter creates a reporter that prints a summary every interval
// based on events from the given EventLog.
func NewConsoleReporter(eventLog *eventdb.EventLog, interval time.Duration) *ConsoleReporter {
	return &ConsoleReporter{
		eventLog: eventLog,
		interval: interval,
	}
}

// Start begins the background reporting loop.
func (r *ConsoleReporter) Start() {
	r.stopCh = make(chan struct{})
	r.wg.Add(1)
	go r.loop()
}

// Stop stops the reporter and waits for the goroutine to exit.
func (r *ConsoleReporter) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

func (r *ConsoleReporter) loop() {
	defer r.wg.Done()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.report()
		}
	}
}

// perWorkloadStats holds aggregated metrics for a single workload over a reporting tick.
type perWorkloadStats struct {
	metricsCount int
	totalOps     int64
	totalSuccess int64
	totalFailure int64
	sumP50       int64
	sumP90       int64
	sumP99       int64
	maxLatency   time.Duration
	minTime      time.Time
	maxTime      time.Time
}

func (r *ConsoleReporter) report() {
	events := r.eventLog.Events()
	if r.lastIdx >= len(events) {
		fmt.Println("[METRICS] no new events")
		return
	}

	newEvents := events[r.lastIdx:]
	r.lastIdx = len(events)

	var (
		eventCount         int
		errorCount         int
		metricsCount       int
		workerMetricsCount int

		totalOps     int64
		totalFailure int64

		workerCount  int
		sumWorkerCPU float64
		sumWorkerRSS int64
		dinoCount    int
		sumDinoCPU   float64
		sumDinoRSS   int64
	)

	// Per-workload tracking.
	workloadMap := make(map[string]*perWorkloadStats)

	for _, e := range newEvents {
		switch ev := e.(type) {
		case eventdb.StartEvent, eventdb.EndEvent, eventdb.PhaseEvent, eventdb.ActionEvent:
			eventCount++
		case eventdb.ErrorEvent:
			errorCount++
		case eventdb.MetricsEvent:
			metricsCount++
			s := ev.Summary
			ops := s.SuccessCount + s.FailureCount
			totalOps += ops
			totalFailure += s.FailureCount

			ws, exists := workloadMap[ev.Workload]
			if !exists {
				ws = &perWorkloadStats{}
				workloadMap[ev.Workload] = ws
			}
			ws.metricsCount++
			ws.totalOps += ops
			ws.totalSuccess += s.SuccessCount
			ws.totalFailure += s.FailureCount
			ws.sumP50 += int64(s.P50Latency)
			ws.sumP90 += int64(s.P90Latency)
			ws.sumP99 += int64(s.P99Latency)
			if s.MaxLatency > ws.maxLatency {
				ws.maxLatency = s.MaxLatency
			}
			if ws.minTime.IsZero() || s.Timestamp.Before(ws.minTime) {
				ws.minTime = s.Timestamp
			}
			if s.Timestamp.After(ws.maxTime) {
				ws.maxTime = s.Timestamp
			}
		case eventdb.WorkerMetricsEvent:
			workerMetricsCount++
			workerCount++
			sumWorkerCPU += ev.CPUPercent
			sumWorkerRSS += ev.RSSBytes
		case eventdb.DinoSystemMetricsEvent:
			dinoCount++
			sumDinoCPU += ev.CPUPercent
			sumDinoRSS += ev.RSSBytes
		}
	}

	// General summary line: event counts, total ops, failures, worker.
	fmt.Printf("[METRICS] %d new events (%d event, %d error, %d metrics, %d worker)",
		len(newEvents), eventCount, errorCount, metricsCount, workerMetricsCount)

	if metricsCount > 0 {
		fmt.Printf(" | %d ops, %d failures", totalOps, totalFailure)
	}

	if workerCount > 0 {
		avgCPU := sumWorkerCPU / float64(workerCount)
		avgRSS := sumWorkerRSS / int64(workerCount)
		fmt.Printf(" | worker: cpu %.1f%%, rss %s", avgCPU, formatBytes(avgRSS))
	}

	if dinoCount > 0 {
		avgCPU := sumDinoCPU / float64(dinoCount)
		avgRSS := sumDinoRSS / int64(dinoCount)
		fmt.Printf(" | dinotest: cpu %.1f%%, rss %s", avgCPU, formatBytes(avgRSS))
	}

	fmt.Println()

	// Per-workload detail lines, sorted lexicographically.
	workloadNames := make([]string, 0, len(workloadMap))
	for name := range workloadMap {
		workloadNames = append(workloadNames, name)
	}
	sort.Strings(workloadNames)

	for _, name := range workloadNames {
		ws := workloadMap[name]
		n := int64(ws.metricsCount)

		// Each MetricsEvent covers a full 1-second bucket, so the
		// actual time span is (last - first) + 1 second.
		var opsPerSec float64
		if ws.metricsCount > 0 {
			span := ws.maxTime.Sub(ws.minTime) + time.Second
			opsPerSec = float64(ws.totalOps) / span.Seconds()
		}

		avgP50 := time.Duration(ws.sumP50 / n)
		avgP90 := time.Duration(ws.sumP90 / n)
		avgP99 := time.Duration(ws.sumP99 / n)

		fmt.Printf("  [%s] %d ops (%.1f/s) | ok: %d  fail: %d | p50 %s, p90 %s, p99 %s, max %s\n",
			name, ws.totalOps, opsPerSec,
			ws.totalSuccess, ws.totalFailure,
			avgP50.Truncate(time.Microsecond),
			avgP90.Truncate(time.Microsecond),
			avgP99.Truncate(time.Microsecond),
			ws.maxLatency.Truncate(time.Microsecond),
		)
	}
}
