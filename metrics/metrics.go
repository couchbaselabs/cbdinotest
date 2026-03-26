package metrics

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/couchbaselabs/cbdinotest/eventdb"
)

// MetricsError represents an error with a code and details.
type MetricsError struct {
	Code    string
	Details string
}

func (e *MetricsError) Error() string {
	if e.Details != "" {
		return e.Code + ": " + e.Details
	}
	return e.Code
}

// --- secondBucket (internal) ---

// errorEntry tracks per-code error data within a single second.
type errorEntry struct {
	details string
	count   int64
}

type secondBucket struct {
	timestamp    time.Time
	successes    int64
	failures     int64
	latencies    []time.Duration
	totalNs      int64
	minLatency   time.Duration
	maxLatency   time.Duration
	errorsByCode map[string]*errorEntry
	pendingOps   int64
}

func newSecondBucket(ts time.Time) *secondBucket {
	return &secondBucket{
		timestamp:    ts,
		latencies:    make([]time.Duration, 0, 256),
		minLatency:   time.Duration(math.MaxInt64),
		errorsByCode: make(map[string]*errorEntry),
	}
}

func (b *secondBucket) add(d time.Duration, opErr *MetricsError) {
	b.latencies = append(b.latencies, d)
	b.totalNs += int64(d)

	if d < b.minLatency {
		b.minLatency = d
	}
	if d > b.maxLatency {
		b.maxLatency = d
	}

	if opErr != nil {
		b.failures++
		entry, exists := b.errorsByCode[opErr.Code]
		if !exists {
			b.errorsByCode[opErr.Code] = &errorEntry{
				details: opErr.Details,
				count:   1,
			}
		} else {
			entry.count++
		}
	} else {
		b.successes++
	}
}

func (b *secondBucket) finalize() eventdb.SecondSummary {
	n := int64(len(b.latencies))
	if n == 0 {
		return eventdb.SecondSummary{Timestamp: b.timestamp}
	}

	sort.Slice(b.latencies, func(i, j int) bool {
		return b.latencies[i] < b.latencies[j]
	})

	return eventdb.SecondSummary{
		Timestamp:    b.timestamp,
		SuccessCount: b.successes,
		FailureCount: b.failures,
		AvgLatency:   time.Duration(b.totalNs / n),
		MinLatency:   b.minLatency,
		MaxLatency:   b.maxLatency,
		P50Latency:   percentile(b.latencies, 0.50),
		P90Latency:   percentile(b.latencies, 0.90),
		P99Latency:   percentile(b.latencies, 0.99),
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}

// --- PendingOp ---

// PendingOp is an opaque handle returned by Collector.StartOp. It
// pins the operation to the second bucket that was active at start
// time and captures the precise start time for duration measurement.
// Pass it back to CompleteOp when the operation finishes.
type PendingOp struct {
	bucket    *secondBucket
	startTime time.Time
}

// --- Collector ---

// Collector aggregates per-second metrics. Operations are pinned to
// the second bucket active at their start time via StartOp, and
// results are written back to that bucket via CompleteOp.
//
// The current-second bucket is kept as a fast-path pointer. When the
// second rolls over, the old bucket moves to pendingBuckets (if it
// still has in-flight ops) or is finalized immediately. Pending
// buckets are drained in FIFO order once their pending ops reach zero.
type Collector struct {
	mu             sync.Mutex
	current        *secondBucket
	pendingBuckets []*secondBucket
	eventLog       *eventdb.EventLog
	workloadName   string
	finalized      bool
}

// ErrFinalized is returned by StartOp when the collector has been finalized.
var ErrFinalized = errors.New("metrics collector has been finalized")

// NewCollector creates a new Collector that appends MetricsEvents to
// the given EventLog, tagged with the given workload name.
// Pass nil for eventLog if no event log is needed.
func NewCollector(workloadName string, eventLog *eventdb.EventLog) *Collector {
	return &Collector{
		eventLog:     eventLog,
		workloadName: workloadName,
	}
}

// StartOp marks the beginning of an operation. The returned PendingOp
// is pinned to the current second's bucket. The caller must pass it
// to CompleteOp when the operation finishes.
// Returns ErrFinalized if the collector has been finalized.
func (c *Collector) StartOp() (PendingOp, error) {
	now := time.Now()
	ts := now.Truncate(time.Second)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.finalized {
		return PendingOp{}, ErrFinalized
	}

	// Fast path: current bucket matches the current second.
	if c.current != nil && c.current.timestamp.Equal(ts) {
		c.current.pendingOps++
		return PendingOp{bucket: c.current, startTime: now}, nil
	}

	// Second has rolled over (or first call). Retire the old current.
	if c.current != nil {
		c.retireCurrentLocked()
	}

	c.current = newSecondBucket(ts)
	c.current.pendingOps++
	return PendingOp{bucket: c.current, startTime: now}, nil
}

// CompleteOp records the result of a previously started operation. The
// elapsed duration is computed from the start time captured in the
// PendingOp. The result is written to the bucket that was active when
// StartOp was called, regardless of how much wall-clock time has elapsed.
func (c *Collector) CompleteOp(op PendingOp, opErr *MetricsError) {
	elapsed := time.Since(op.startTime)

	c.mu.Lock()
	defer c.mu.Unlock()

	op.bucket.add(elapsed, opErr)
	op.bucket.pendingOps--

	c.drainReadyBucketsLocked()
}

// Finalize flushes all buckets (current and pending) to the EventLog
// and marks the collector as finalized. Any subsequent calls to
// StartOp will return ErrFinalized.
func (c *Collector) Finalize() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Drain all pending buckets unconditionally.
	for _, b := range c.pendingBuckets {
		c.emitBucketLocked(b)
	}
	c.pendingBuckets = nil

	// Finalize the current bucket.
	if c.current != nil {
		c.emitBucketLocked(c.current)
		c.current = nil
	}

	c.finalized = true
}

// retireCurrentLocked moves the current bucket to pendingBuckets if it
// still has in-flight ops, or finalizes it immediately if it doesn't.
// Must be called with c.mu held.
func (c *Collector) retireCurrentLocked() {
	if c.current.pendingOps > 0 {
		c.pendingBuckets = append(c.pendingBuckets, c.current)
	} else {
		c.emitBucketLocked(c.current)
		// Also drain any pending buckets that may now be ready,
		// in case the retired bucket was blocking the queue.
		c.drainReadyBucketsLocked()
	}
}

// drainReadyBucketsLocked finalizes pending buckets from the front of
// the slice as long as they have zero pending ops. Stops at the first
// bucket that still has in-flight operations (preserving serial order).
// Must be called with c.mu held.
func (c *Collector) drainReadyBucketsLocked() {
	i := 0
	for i < len(c.pendingBuckets) && c.pendingBuckets[i].pendingOps == 0 {
		c.emitBucketLocked(c.pendingBuckets[i])
		i++
	}
	if i > 0 {
		c.pendingBuckets = c.pendingBuckets[i:]
	}
}

// emitBucketLocked finalizes a bucket and sends it to the EventLog.
// Must be called with c.mu held.
func (c *Collector) emitBucketLocked(bucket *secondBucket) {
	if bucket.pendingOps > 0 {
		fmt.Printf("[METRICS] WARNING: finalizing bucket %s with %d pending ops still in flight\n",
			bucket.timestamp.Format("15:04:05"), bucket.pendingOps)
	}

	summary := bucket.finalize()

	if c.eventLog != nil {
		c.eventLog.Add(eventdb.MetricsEvent{Workload: c.workloadName, Summary: summary})
		for code, entry := range bucket.errorsByCode {
			c.eventLog.Add(eventdb.ErrorEvent{
				Time:     bucket.timestamp,
				Workload: c.workloadName,
				Code:     code,
				Details:  entry.details,
				Count:    entry.count,
			})
		}
	}
}
