package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initializes cbdinotest resources.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("init complete")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
