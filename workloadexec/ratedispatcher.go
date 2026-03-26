package workloadexec

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/couchbaselabs/cbdinotest/jsexec"
	"github.com/couchbaselabs/cbdinotest/metrics"
)

// RateDispatcher dispatches workload executions at a fixed rate,
// independent of how long each execution takes. This avoids
// coordinated omission — slow requests do not reduce the dispatch
// rate, and the measured latency distribution accurately reflects
// the true cost of each operation. Instances are acquired from
// the WorkloadPool.
type RateDispatcher struct {
	pool      *WorkloadPool
	rate      float64 // requests per second
	collector *metrics.Collector

	inflight int64 // atomic: number of in-flight goroutines

	stopCh chan struct{}
	doneCh chan struct{}
	wg     sync.WaitGroup // tracks in-flight dispatch goroutines
}

// NewRateDispatcher creates a dispatcher that will fire requests at
// the given rate (ops/sec) using instances from the WorkloadPool.
func NewRateDispatcher(
	pool *WorkloadPool,
	rate float64,
	collector *metrics.Collector,
) *RateDispatcher {
	return &RateDispatcher{
		pool:      pool,
		rate:      rate,
		collector: collector,
	}
}

// Start begins the dispatch loop. It fires one request every 1/rate
// seconds. Each request runs in its own goroutine using a pooled
// instance. Start returns immediately.
func (rd *RateDispatcher) Start() {
	rd.stopCh = make(chan struct{})
	rd.doneCh = make(chan struct{})

	interval := time.Duration(float64(time.Second) / rd.rate)

	go func() {
		defer close(rd.doneCh)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-rd.stopCh:
				rd.wg.Wait()
				return
			case <-ticker.C:
				rd.dispatch()
			}
		}
	}()
}

// Stop signals the dispatcher to stop and waits for all in-flight
// requests to complete.
func (rd *RateDispatcher) Stop() {
	close(rd.stopCh)
	<-rd.doneCh
}

// Done returns a channel that is closed when the dispatcher has
// fully stopped (all in-flight requests complete).
func (rd *RateDispatcher) Done() <-chan struct{} {
	return rd.doneCh
}

// Inflight returns the current number of in-flight requests.
func (rd *RateDispatcher) Inflight() int64 {
	return atomic.LoadInt64(&rd.inflight)
}

// dispatch acquires an instance from the pool and runs a single
// execution in a new goroutine, releasing the instance when done.
func (rd *RateDispatcher) dispatch() {
	rd.wg.Add(1)
	atomic.AddInt64(&rd.inflight, 1)

	// Pin this operation to the current second's bucket before dispatch.
	op, err := rd.collector.StartOp()
	if err != nil {
		rd.wg.Done()
		atomic.AddInt64(&rd.inflight, -1)
		return
	}

	go func() {
		defer rd.wg.Done()
		defer atomic.AddInt64(&rd.inflight, -1)

		exec, err := rd.pool.Acquire()
		if err != nil {
			fmt.Printf("[RATE-DISPATCH] failed to acquire instance: %s\n", err)
			rd.collector.CompleteOp(op, nil)
			return
		}

		execErr := exec.Execute()

		if execErr != nil {
			if jsexec.IsInterrupt(execErr) {
				rd.collector.CompleteOp(op, nil)
				rd.pool.Release(exec)
				return
			}

			code, details := jsErrorToMetricsError(exec, execErr)
			rd.collector.CompleteOp(op, &metrics.MetricsError{Code: code, Details: details})
		} else {
			rd.collector.CompleteOp(op, nil)
		}

		rd.pool.Release(exec)
	}()
}
