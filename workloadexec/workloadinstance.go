package workloadexec

import (
	"sync"
	"time"

	"github.com/couchbaselabs/cbdinotest/jsexec"
	"github.com/couchbaselabs/cbdinotest/metrics"
)

// WorkloadInstance is a single runnable instance of a workload.
// It owns the run loop and metrics collection, delegating JS
// execution to a WorkloadExec (which holds its own setup data).
type WorkloadInstance struct {
	exec      *WorkloadExec
	collector *metrics.Collector

	mu      sync.Mutex
	stopCh  chan struct{}
	doneCh  chan struct{}
	stopped bool
}

// NewWorkloadInstance creates a WorkloadInstance from a WorkloadExec
// and a metrics collector.
func NewWorkloadInstance(exec *WorkloadExec, collector *metrics.Collector) *WorkloadInstance {
	return &WorkloadInstance{
		exec:      exec,
		collector: collector,
	}
}

// Info returns the workload metadata.
func (wi *WorkloadInstance) Info() WorkloadInfo {
	return wi.exec.Info()
}

// Done returns a channel that is closed when the instance's run loop
// has exited, whether due to an error or a graceful stop.
func (wi *WorkloadInstance) Done() <-chan struct{} {
	wi.mu.Lock()
	defer wi.mu.Unlock()
	return wi.doneCh
}

// Run starts the execution loop in a background goroutine. Each
// iteration calls the default export with the exec's internal setup
// data. Run returns immediately.
func (wi *WorkloadInstance) Run() {
	exec := wi.exec

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	wi.mu.Lock()
	wi.stopCh = stopCh
	wi.doneCh = doneCh
	wi.stopped = false
	wi.mu.Unlock()

	collector := wi.collector
	go func() {
		defer close(doneCh)

	RunLoop:
		for {
			select {
			case <-stopCh:
				break RunLoop
			default:
			}

			op, err := collector.StartOp()
			if err != nil {
				break RunLoop
			}

			iterErr := exec.Execute()

			if iterErr != nil {
				if jsexec.IsInterrupt(iterErr) {
					collector.CompleteOp(op, nil)
					break RunLoop
				}

				code, details := jsErrorToMetricsError(exec, iterErr)
				collector.CompleteOp(op, &metrics.MetricsError{Code: code, Details: details})
				continue
			}

			collector.CompleteOp(op, nil)
		}
	}()
}

// Stop signals the workload instance to stop. It waits up to 10 seconds
// for the current iteration to complete gracefully. If the iteration
// does not finish in time, the Goja VM is interrupted to force it to stop.
func (wi *WorkloadInstance) Stop() {
	wi.mu.Lock()
	if wi.stopped {
		wi.mu.Unlock()
		return
	}
	wi.stopped = true
	stopCh := wi.stopCh
	doneCh := wi.doneCh
	wi.mu.Unlock()

	close(stopCh)

	select {
	case <-doneCh:
		return
	case <-time.After(10 * time.Second):
	}

	wi.exec.Interrupt("workload stop timeout")
	<-doneCh
}
