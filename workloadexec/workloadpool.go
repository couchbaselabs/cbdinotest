package workloadexec

import (
	"fmt"
	"sync"
	"time"

	"github.com/couchbaselabs/cbdinotest/metrics"
)

// WorkloadPool maintains a reusable pool of cloned WorkloadExec
// instances and manages concurrency-based WorkloadInstances. Clone
// automatically carries the worker URI and setup data.
type WorkloadPool struct {
	workload *Workload // source for cloning

	mu        sync.Mutex
	pool      []*WorkloadExec     // idle instances (for rate dispatch)
	instances []*WorkloadInstance // running concurrency instances
}

// NewWorkloadPool creates a pool that clones instances from the given
// workload. The workload's exec must have had SetWorkerURI and Setup
// called so that Clone() automatically carries both.
func NewWorkloadPool(workload *Workload) *WorkloadPool {
	return &WorkloadPool{
		workload: workload,
		pool:     make([]*WorkloadExec, 0, 16),
	}
}

// RunInstance creates a new WorkloadExec clone, wraps it in a
// WorkloadInstance with the given collector, starts its run loop,
// and tracks it internally. Call Stop() to stop all running
// instances.
func (wp *WorkloadPool) RunInstance(collector *metrics.Collector) error {
	exec, err := wp.clone()
	if err != nil {
		return err
	}

	inst := NewWorkloadInstance(exec, collector)
	inst.Run()

	wp.mu.Lock()
	wp.instances = append(wp.instances, inst)
	wp.mu.Unlock()

	return nil
}

// Acquire returns an idle WorkloadExec from the pool, or creates
// a new one by cloning the workload into a fresh goja VM.
// The worker URI and setup data are automatically carried by Clone.
func (wp *WorkloadPool) Acquire() (*WorkloadExec, error) {
	wp.mu.Lock()
	if len(wp.pool) > 0 {
		inst := wp.pool[len(wp.pool)-1]
		wp.pool = wp.pool[:len(wp.pool)-1]
		wp.mu.Unlock()
		return inst, nil
	}
	wp.mu.Unlock()

	return wp.clone()
}

// Release returns a WorkloadExec to the pool for reuse.
func (wp *WorkloadPool) Release(exec *WorkloadExec) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.pool = append(wp.pool, exec)
}

// Stop stops all concurrency-mode instances managed by this pool.
// Each instance is given 10 seconds to finish gracefully before
// its VM is interrupted.
func (wp *WorkloadPool) Stop() {
	wp.mu.Lock()
	instances := wp.instances
	wp.instances = nil
	wp.mu.Unlock()

	for _, inst := range instances {
		inst.Stop()
	}
}

// Done returns a channel that is closed when all concurrency-mode
// instances managed by this pool have exited.
func (wp *WorkloadPool) Done() <-chan struct{} {
	wp.mu.Lock()
	instances := wp.instances
	wp.mu.Unlock()

	ch := make(chan struct{})
	go func() {
		for _, inst := range instances {
			select {
			case <-inst.Done():
			case <-time.After(30 * time.Second):
			}
		}
		close(ch)
	}()
	return ch
}

// clone creates a new WorkloadExec from the workload's compiled program.
func (wp *WorkloadPool) clone() (*WorkloadExec, error) {
	cloned, err := wp.workload.exec.Clone()
	if err != nil {
		return nil, fmt.Errorf("failed to clone workload: %w", err)
	}
	return cloned, nil
}
