package workloadexec

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/couchbaselabs/cbdinotest/eventdb"
	"github.com/couchbaselabs/cbdinotest/metrics"
)

// WorkloadRunnerConfig describes a workload to run, including its
// source file path and how to dispatch operations. Set Concurrency
// for N tight-loop goroutines, or Rate for timer-based dispatch at
// the given ops/sec (avoids coordinated omission). If both are zero,
// Concurrency defaults to 1.
type WorkloadRunnerConfig struct {
	FilePath    string
	Concurrency int
	Rate        float64
}

// workloadEntry holds a compiled workload, its name, and its dispatch settings.
type workloadEntry struct {
	name        string
	workload    *Workload
	concurrency int
	rate        float64
}

// runningEntry holds per-workload runtime state created during Start().
type runningEntry struct {
	workload  *Workload
	pool      *WorkloadPool
	collector *metrics.Collector
}

// WorkloadRunner manages a set of compiled workloads and their instances.
// It implements jsexec.WorkloadRunner.
type WorkloadRunner struct {
	entries  []workloadEntry
	worker   *WorkerClient
	eventLog *eventdb.EventLog

	mu              sync.Mutex
	runningEntries  []runningEntry
	rateDispatchers []*RateDispatcher
	setupDone       bool
	running         bool
	metricsStopCh   chan struct{}
}

// NewWorkloadRunner accepts a map of workload configs keyed by name,
// a WorkerClient, and an optional event log. If eventLog is nil, a new
// one is created internally. Bundles and compiles each workload
// script, then returns a runner ready to start.
func NewWorkloadRunner(configs map[string]WorkloadRunnerConfig, worker *WorkerClient, eventLog *eventdb.EventLog) (*WorkloadRunner, error) {
	if eventLog == nil {
		eventLog = eventdb.NewEventLog()
	}
	entries := make([]workloadEntry, 0, len(configs))

	for name, cfg := range configs {
		w, err := NewWorkload(cfg.FilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to create workload %q: %w", name, err)
		}

		entry := workloadEntry{
			name:     name,
			workload: w,
		}

		if cfg.Rate > 0 {
			entry.rate = cfg.Rate
			fmt.Printf("[RUNNER] loaded workload %q (%s) with rate=%.1f ops/s\n",
				name, cfg.FilePath, cfg.Rate)
		} else {
			concurrency := cfg.Concurrency
			if concurrency <= 0 {
				concurrency = 1
			}
			entry.concurrency = concurrency
			fmt.Printf("[RUNNER] loaded workload %q (%s) with concurrency=%d\n",
				name, cfg.FilePath, concurrency)
		}

		entries = append(entries, entry)
	}

	return &WorkloadRunner{
		entries:  entries,
		worker:   worker,
		eventLog: eventLog,
	}, nil
}

// Setup calls /_setup on the worker to initialize the SDK connection.
// This should be called before the cluster settle wait so that assets
// like indexes created by the worker have time to build.
func (r *WorkloadRunner) Setup(setupOpts interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.setupDone {
		return fmt.Errorf("workload runner is already set up")
	}

	setupOptsJSON, _ := json.Marshal(setupOpts)
	fmt.Printf("[RUNNER] calling /_setup with setupOpts=%s\n", setupOptsJSON)
	if err := r.worker.Setup(setupOpts); err != nil {
		return fmt.Errorf("worker setup failed: %w", err)
	}

	r.setupDone = true
	return nil
}

// Start runs setup() on each workload, creates a WorkloadPool per
// workload, and starts the appropriate dispatchers. Setup must have
// been called first.
func (r *WorkloadRunner) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.setupDone {
		return fmt.Errorf("workload runner has not been set up, call Setup first")
	}
	if r.running {
		return fmt.Errorf("workload runner is already running")
	}

	// Run setup() once for each workload and create a pool per workload.
	var runningEntries []runningEntry
	for _, entry := range r.entries {
		if err := entry.workload.Setup(r.worker.baseURL); err != nil {
			return fmt.Errorf("setup failed for workload %q: %w", entry.name, err)
		}

		pool := NewWorkloadPool(entry.workload)
		collector := metrics.NewCollector(entry.name, r.eventLog)
		runningEntries = append(runningEntries, runningEntry{
			workload:  entry.workload,
			pool:      pool,
			collector: collector,
		})
	}

	var rateDispatchers []*RateDispatcher
	var startErr error

StartLoop:
	for i, entry := range r.entries {
		re := runningEntries[i]
		if entry.rate > 0 {
			// Rate-based dispatch mode.
			rd := NewRateDispatcher(re.pool, entry.rate, re.collector)
			rd.Start()
			rateDispatchers = append(rateDispatchers, rd)
		} else {
			// Concurrency-based dispatch mode.
			for j := 0; j < entry.concurrency; j++ {
				if err := re.pool.RunInstance(re.collector); err != nil {
					startErr = fmt.Errorf("failed to create instance of %q: %w",
						entry.workload.Info().Name, err)
					break StartLoop
				}
			}
		}
	}

	if startErr != nil {
		for _, rd := range rateDispatchers {
			rd.Stop()
		}
		for _, re := range runningEntries {
			re.pool.Stop()
		}
		return startErr
	}

	r.runningEntries = runningEntries
	r.rateDispatchers = rateDispatchers
	r.running = true

	// Start background worker metrics polling.
	r.metricsStopCh = make(chan struct{})
	r.startMetricsPoller()

	fmt.Printf("[RUNNER] started %d rate dispatchers across %d workloads\n",
		len(rateDispatchers), len(r.entries))

	return nil
}

// Stop stops all rate dispatchers and workload pools, calls
// teardown once per workload, then calls /_cleanup on the worker.
func (r *WorkloadRunner) Stop() error {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return fmt.Errorf("workload runner is not running")
	}
	rateDispatchers := r.rateDispatchers
	runningEntries := r.runningEntries
	r.rateDispatchers = nil
	r.runningEntries = nil
	r.running = false
	r.mu.Unlock()

	fmt.Printf("[RUNNER] stopping %d rate dispatchers...\n", len(rateDispatchers))

	for _, rd := range rateDispatchers {
		rd.Stop()
	}

	// Stop all pools (this stops all concurrency-mode instances).
	for _, re := range runningEntries {
		re.pool.Stop()
	}

	fmt.Println("[RUNNER] all dispatchers and pools stopped")

	// Finalize each workload's collector (once per workload).
	for _, re := range runningEntries {
		re.collector.Finalize()
	}

	// Teardown each workload once (not per-instance).
	for _, re := range runningEntries {
		if err := re.workload.Teardown(); err != nil {
			fmt.Printf("[RUNNER] teardown error for %q: %s\n",
				re.workload.Info().Name, err)
		}
	}

	// Stop the background metrics poller.
	close(r.metricsStopCh)

	// Clean up the worker after all instances have stopped.
	fmt.Println("[RUNNER] calling /_cleanup")
	if err := r.worker.Cleanup(); err != nil {
		return fmt.Errorf("worker cleanup failed: %w", err)
	}

	return nil
}

// Done returns a channel that is closed when all rate dispatchers
// and pool-managed instances have exited.
func (r *WorkloadRunner) Done() <-chan struct{} {
	r.mu.Lock()
	rateDispatchers := r.rateDispatchers
	runningEntries := r.runningEntries
	r.mu.Unlock()

	ch := make(chan struct{})
	go func() {
		for _, rd := range rateDispatchers {
			<-rd.Done()
		}
		for _, re := range runningEntries {
			<-re.pool.Done()
		}
		close(ch)
	}()
	return ch
}

// EventLog returns the shared event log used by all workload instances.
func (r *WorkloadRunner) EventLog() *eventdb.EventLog {
	return r.eventLog
}

// startMetricsPoller captures an initial /_metrics baseline, then
// polls the worker every second and pushes WorkerMetricsEvent to the
// event log. Stops when metricsStopCh is closed.
func (r *WorkloadRunner) startMetricsPoller() {
	// Capture the initial baseline (primes the CPU delta on the worker side).
	r.worker.Metrics()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.metricsStopCh:
				return
			case <-ticker.C:
				m, err := r.worker.Metrics()
				if err == nil {
					r.eventLog.Add(eventdb.WorkerMetricsEvent{
						Time:       time.Now(),
						CPUPercent: m.CPUPercent,
						RSSBytes:   m.RSSBytes,
					})
				}
			}
		}
	}()
}
