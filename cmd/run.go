package cmd

import (
	"log"
	"os"
	"strings"

	"github.com/couchbaselabs/cbdinotest/situationexec"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run [workload file]",
	Short: "Runs the test",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		workerAddr, _ := cmd.Flags().GetString("worker")
		paramFlags, _ := cmd.Flags().GetStringArray("param")
		paramsFlag, _ := cmd.Flags().GetString("params")
		exportDBPath, _ := cmd.Flags().GetString("export-db")
		reuseCbdcIDFlag, _ := cmd.Flags().GetString("reuse-cbdc-id")
		reuseCbdcIDChanged := cmd.Flags().Changed("reuse-cbdc-id")

		// Canonicalize the worker URL, defaulting to http:// if no scheme is provided.
		worker := workerAddr
		if !strings.Contains(workerAddr, "://") {
			worker = "http://" + workerAddr
		}

		params := make(map[string]string)

		parseParam := func(p string) {
			parts := strings.SplitN(p, "=", 2)
			if len(parts) != 2 {
				log.Fatalf("invalid parameter format %q, expected key=value", p)
			}
			params[parts[0]] = parts[1]
		}

		for _, p := range paramFlags {
			parseParam(p)
		}

		if paramsFlag != "" {
			for _, p := range strings.Split(paramsFlag, ",") {
				parseParam(p)
			}
		}

		sit, err := situationexec.NewSituation(args[0])
		if err != nil {
			log.Fatalf("failed to load situation: %s", err)
		}

		runOpts := situationexec.RunOptions{
			ExportDBPath: exportDBPath,
		}

		// --reuse-cbdc-id was explicitly provided (possibly with empty value)
		if reuseCbdcIDChanged {
			trimmed := strings.TrimSpace(reuseCbdcIDFlag)
			runOpts.ReuseCbdcID = &trimmed
		}

		passed, err := sit.Run(params, worker, runOpts)
		if err != nil {
			log.Fatalf("situation failed: %s", err)
		}
		if !passed {
			os.Exit(1)
		}
	},
}

func init() {
	runCmd.Flags().String("worker", "localhost:4000", "testsvc worker address (host:port)")
	runCmd.Flags().StringArrayP("param", "p", nil, "Parameter in key=value format (can be specified multiple times)")
	runCmd.Flags().String("params", "", "Comma-delimited parameters in key=value,key=value format")
	runCmd.Flags().String("export-db", "", "Export the event log to the specified file path at the end of the run")
	runCmd.Flags().String("reuse-cbdc-id", "", "Reuse an existing dinocluster; omit value to allocate-and-keep, or pass a cluster ID to reuse")
	runCmd.Flags().Lookup("reuse-cbdc-id").NoOptDefVal = " "
	rootCmd.AddCommand(runCmd)
}
