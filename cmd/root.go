package cmd

import (
	"log"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "cbdinotest",
	Short: "provides tooling for quickly executing situational tests.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("failed to initialize command line parser: %s", err)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Turns on verbose logging")
	rootCmd.PersistentFlags().Bool("json", false, "Turns on JSON output for supported commands")
}
