package cmd

import (
	"fmt"

	"github.com/couchbaselabs/cbdinotest/contrib/buildversion"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Aliases: []string{"ver"},
	Short:   "Gets the version of cbdinotest",
	Run: func(cmd *cobra.Command, args []string) {
		version := buildversion.GetVersion("github.com/couchbaselabs/cbdinotest")
		fmt.Printf("%s\n", version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
