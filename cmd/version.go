package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version information
var (
	Commit    string
	BuildTime string
	Version   string
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Version:\t%s\nRevision:\t%s\nBuild time:\t%s\n", Version, Commit, BuildTime)
	},
}
