package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/couchbaselabs/cbdinotest/metrics"
	"github.com/couchbaselabs/cbdinotest/workloadexec"
	"github.com/spf13/cobra"
)

var runWorkloadCmd = &cobra.Command{
	Use:   "run-workload [workload file]",
	Short: "Runs a single workload directly",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		workerAddr, _ := cmd.Flags().GetString("worker")
		concurrency, _ := cmd.Flags().GetInt("concurrency")
		rate, _ := cmd.Flags().GetFloat64("rate")
		setupJSON, _ := cmd.Flags().GetString("setup")

		if cmd.Flags().Changed("concurrency") && cmd.Flags().Changed("rate") {
			log.Fatalf("cannot specify both --concurrency and --rate")
		}

		// When rate is specified, clear concurrency so the runner uses rate mode.
		if rate > 0 {
			concurrency = 0
		}

		// Canonicalize the worker URL, defaulting to http:// if no scheme is provided.
		worker := workerAddr
		if !strings.Contains(workerAddr, "://") {
			worker = "http://" + workerAddr
		}

		// Parse the setup JSON payload if provided.
		var setupOpts interface{}
		if setupJSON != "" {
			if err := json.Unmarshal([]byte(setupJSON), &setupOpts); err != nil {
				log.Fatalf("invalid --setup JSON: %s", err)
			}
		}

		// Use the workload file basename (without extension) as the workload name.
		filePath := args[0]

		configs := map[string]workloadexec.WorkloadRunnerConfig{
			"default": {
				FilePath:    filePath,
				Concurrency: concurrency,
				Rate:        rate,
			},
		}

		workerClient := workloadexec.NewWorkerClient(worker)

		runner, err := workloadexec.NewWorkloadRunner(configs, workerClient, nil)
		if err != nil {
			log.Fatalf("failed to create workload runner: %s", err)
		}

		if err := runner.Setup(setupOpts); err != nil {
			log.Fatalf("failed to set up workload runner: %s", err)
		}

		if err := runner.Start(); err != nil {
			log.Fatalf("failed to start workload runner: %s", err)
		}

		// Start a console reporter to print metrics every 10 seconds.
		reporter := metrics.NewConsoleReporter(runner.EventLog(), 5*time.Second)
		reporter.Start()

		// Wait for either a signal or all instances to exit.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		firstSigTime := time.Time{}
	waitLoop:
		for {
			select {
			case sig := <-sigCh:
				if !firstSigTime.IsZero() && time.Since(firstSigTime) >= 500*time.Millisecond {
					fmt.Printf("\n[SIGNAL] received second %s, force exiting...\n", sig)
					os.Exit(1)
				}
				if firstSigTime.IsZero() {
					firstSigTime = time.Now()
				}
				fmt.Printf("\n[SIGNAL] received %s, stopping workloads...\n", sig)
				break waitLoop
			case <-runner.Done():
				fmt.Println("\n[RUNNER] all workload instances have exited")
				break waitLoop
			}
		}

		reporter.Stop()

		if err := runner.Stop(); err != nil {
			log.Fatalf("failed to stop workload runner: %s", err)
		}

		metrics.PrintRunSummary("WORKLOAD SUMMARY", runner.EventLog(), metrics.PrintSummaryOptions{})
	},
}

func init() {
	runWorkloadCmd.Flags().String("worker", "localhost:4000", "testsvc worker address (host:port)")
	runWorkloadCmd.Flags().Int("concurrency", 1, "number of concurrent workload instances (mutually exclusive with --rate)")
	runWorkloadCmd.Flags().Float64("rate", 0, "target request rate in ops/sec (mutually exclusive with --concurrency)")
	runWorkloadCmd.Flags().String("setup", "", "JSON payload to pass to the worker /_setup endpoint")
	rootCmd.AddCommand(runWorkloadCmd)
}
