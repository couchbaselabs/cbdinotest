package situationexec

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/couchbaselabs/cbdinotest/cbdino"
	"github.com/couchbaselabs/cbdinotest/eventdb"
	"github.com/couchbaselabs/cbdinotest/jsexec"
	"github.com/couchbaselabs/cbdinotest/metrics"
	"github.com/couchbaselabs/cbdinotest/scoring"
	"github.com/couchbaselabs/cbdinotest/workloadexec"
	"github.com/google/uuid"
)

// RunOptions holds optional parameters for Situation.Run.
type RunOptions struct {
	ExportDBPath string
	ReuseCbdcID  *string // nil=normal, ""=allocate-and-keep, "ID"=reuse
}

type SituationParam struct {
	Type    string
	Default interface{}   // nil = no default; int, float64, or string
	Choices []interface{} // each element is int, float64, or string
}

// WorkloadPhaseScoring defines scoring criteria for a specific phase.
type WorkloadPhaseScoring struct {
	MinSuccessRate  *float64
	MaxSuccessRate  *float64
	MinOpsPerSecond *float64
}

// SituationWorkload describes a workload to run during a situation,
// including its path, dispatch mode, and per-phase scoring criteria.
type SituationWorkload struct {
	Path        string
	Concurrency int
	Rate        float64
	Scoring     map[string]WorkloadPhaseScoring
}

type SituationInfo struct {
	Name        string
	Description string
	Params      map[string]SituationParam
	Workloads   map[string]SituationWorkload
}

// Situation holds a pre-compiled situation and orchestrates the
// business logic of running it: creating controllers, workload
// runners, handling signals, scoring, and reporting.
type Situation struct {
	exec *SituationExec
}

// NewSituation bundles and compiles the situation script at filePath.
func NewSituation(filePath string) (*Situation, error) {
	exec, err := NewSituationExec(filePath)
	if err != nil {
		return nil, err
	}

	return &Situation{
		exec: exec,
	}, nil
}

// Info returns the situation metadata extracted during construction.
func (s *Situation) Info() *SituationInfo {
	return s.exec.Info()
}

// Run executes the situation with the given parameters and worker address.
func (s *Situation) Run(args map[string]string, workerAddr string, opts RunOptions) (bool, error) {
	ctrl, err := cbdino.NewController(cbdino.ControllerOptions{
		ReuseCbdcID: opts.ReuseCbdcID,
	})
	if err != nil {
		return false, fmt.Errorf("failed to initialize cbdino controller: %w", err)
	}

	// Create a shared event log for the entire situation run.
	eventLog := eventdb.NewEventLog()

	// Generate a unique run ID and record it immediately after the start event.
	runID := uuid.New().String()
	eventLog.Add(eventdb.RunInfoEvent{Time: time.Now(), RunID: runID})

	// Start self-metrics collection for the entire situation duration.
	selfMetricsStopCh := make(chan struct{})
	selfCollector := metrics.NewSelfMetricsCollector()
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-selfMetricsStopCh:
				return
			case <-ticker.C:
				cpu, rss := selfCollector.Sample()
				eventLog.Add(eventdb.DinoSystemMetricsEvent{
					Time:       time.Now(),
					CPUPercent: cpu,
					RSSBytes:   rss,
				})
			}
		}
	}()

	// Create the worker client and immediately verify it is reachable by
	// calling /_cleanup. This surfaces connectivity problems before we
	// spend time allocating clusters or running any situation logic.
	worker := workloadexec.NewWorkerClient(workerAddr)
	fmt.Println("[RUNNER] calling /_cleanup (pre-setup)")
	if err := worker.Cleanup(); err != nil {
		// A "not_setup" error is expected here — the worker hasn't been
		// initialized yet so there's nothing to clean up. Any other error
		// indicates a real connectivity or worker problem.
		var codedErr *workloadexec.WorkerCodedError
		if !(errors.As(err, &codedErr) && codedErr.Code == "not_setup") {
			close(selfMetricsStopCh)
			return false, fmt.Errorf("worker pre-cleanup failed (is the worker running?): %w", err)
		}
	}

	// Build workload runner configs from the situation's workload options.
	info := s.exec.Info()
	workloadConfigs := make(map[string]workloadexec.WorkloadRunnerConfig)
	for name, wl := range info.Workloads {
		workloadConfigs[name] = workloadexec.WorkloadRunnerConfig{
			FilePath:    wl.Path,
			Concurrency: wl.Concurrency,
			Rate:        wl.Rate,
		}
	}

	workloadRunner, err := workloadexec.NewWorkloadRunner(workloadConfigs, worker, eventLog)
	if err != nil {
		close(selfMetricsStopCh)
		return false, fmt.Errorf("failed to create workload runner: %w", err)
	}

	// Listen for SIGINT/SIGTERM so we can interrupt the JS runtime.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		firstSigTime := time.Time{}
		for sig := range sigCh {
			if !firstSigTime.IsZero() && time.Since(firstSigTime) >= 500*time.Millisecond {
				fmt.Printf("\n[SIGNAL] received second %s, force exiting...\n", sig)
				os.Exit(1)
			}
			if firstSigTime.IsZero() {
				firstSigTime = time.Now()
			}
			fmt.Printf("\n[SIGNAL] received %s, interrupting situation...\n", sig)
			s.exec.Interrupt("interrupted by signal: " + sig.String())
		}
	}()
	defer signal.Stop(sigCh)

	// If the situation fails, clean up any clusters that were created.
	var runErr error
	defer func() {
		if runErr != nil {
			fmt.Printf("[CLEANUP] situation failed: %s\n", runErr)
			fmt.Printf("[CLEANUP] removing all created clusters...\n")
			if cleanupErr := ctrl.RemoveAllCreated(); cleanupErr != nil {
				fmt.Printf("[CLEANUP] error during cluster cleanup: %s\n", cleanupErr)
			}
		}

		reuseID := ctrl.ReusableCbdcID()
		if reuseID != "" {
			fmt.Printf("[DINOTEST] cluster %s left running for future reuse (use --reuse-cbdc-id=%s)\n",
				reuseID, reuseID)
		}
	}()

	// Start a console reporter to print metrics every 10 seconds.
	reporter := metrics.NewConsoleReporter(eventLog, 10*time.Second)
	reporter.Start()

	// Execute the situation via SituationExec, providing a setup
	// callback that registers all domain-specific modules.
	runErr = s.exec.Run(args, func(vm *jsexec.Runtime) {
		jsexec.SetupCbdtHttp(vm)
		jsexec.SetupUtils(vm)
		jsexec.SetupCbdtCbdino(vm, ctrl)
		jsexec.SetupWorkload(vm, workloadRunner)
		jsexec.SetupMetrics(vm, eventLog)
		vm.Set("__cbdt_worker_addr", workerAddr)
	})

	reporter.Stop()
	close(selfMetricsStopCh)
	eventLog.Finalize()

	// Export the event log to a file if requested.
	if opts.ExportDBPath != "" {
		f, err := os.Create(opts.ExportDBPath)
		if err != nil {
			return false, fmt.Errorf("failed to create export file: %w", err)
		}
		if err := eventdb.WriteJSON(f, eventLog); err != nil {
			f.Close()
			return false, fmt.Errorf("failed to write export: %w", err)
		}
		if err := f.Close(); err != nil {
			return false, fmt.Errorf("failed to close export file: %w", err)
		}
		fmt.Printf("[EXPORT] event log written to %s\n", opts.ExportDBPath)
	}

	if runErr != nil {
		return false, runErr
	}

	// --- Print summary ---
	metrics.PrintRunSummary("SITUATION SUMMARY", eventLog, metrics.PrintSummaryOptions{
		IncludePhases:  true,
		IncludeActions: true,
	})

	// Build scoring config from workload definitions.
	scoringConfig := make(map[string]scoring.WorkloadScoring)
	for name, wl := range info.Workloads {
		phases := make(map[string]scoring.PhaseScoring)
		for phase, ps := range wl.Scoring {
			phases[phase] = scoring.PhaseScoring{
				MinSuccessRate:  ps.MinSuccessRate,
				MaxSuccessRate:  ps.MaxSuccessRate,
				MinOpsPerSecond: ps.MinOpsPerSecond,
			}
		}
		scoringConfig[name] = scoring.WorkloadScoring{Phases: phases}
	}

	// Score the run against the workload criteria.
	scorer := scoring.NewScorer(scoringConfig)
	phaseSummaries, err := metrics.SummarizeMetrics(eventLog.Events())
	if err != nil {
		return false, fmt.Errorf("failed to summarize metrics for scoring: %w", err)
	}
	passed := scorer.Score(phaseSummaries)

	if passed {
		fmt.Println()
		fmt.Println("========================================")
		fmt.Println("               PASS")
		fmt.Println("========================================")
		fmt.Println()
	} else {
		fmt.Println()
		fmt.Println("========================================")
		fmt.Println("               FAIL")
		fmt.Println("========================================")
		fmt.Println()
	}

	return passed, nil
}
