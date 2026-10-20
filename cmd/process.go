// Copyright 1999-2026. WebPros International GmbH.

package cmd

import (
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List running processes and the directories they watch",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		instances, err := loadInstances()
		if err != nil {
			return err
		}

		return printInstances(cmd.OutOrStdout(), instances)
	},
}

var killCmd = &cobra.Command{
	Use:   "kill <pid|host|path>",
	Short: "Stop the processes matching the pid, remote host, path or path suffix",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		instances, err := loadInstances()
		if err != nil {
			return err
		}

		matches := findInstances(instances, args[0])
		if len(matches) == 0 {
			return fmt.Errorf("no running process matches %q", args[0])
		}

		return terminateInstances(matches)
	},
}

var killallCmd = &cobra.Command{
	Use:   "killall",
	Short: "Stop all running processes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		instances, err := loadInstances()
		if err != nil {
			return err
		}

		if len(instances) == 0 {
			log.Println("no running processes")
			return nil
		}

		return terminateInstances(instances)
	},
}

func printInstances(w io.Writer, instances []instance) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PID\tPATH\tREMOTE HOST")
	for _, inst := range instances {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\n", inst.PID, inst.Path, inst.RemoteHost)
	}
	return tw.Flush()
}

func findInstances(instances []instance, selector string) []instance {
	pid, _ := strconv.Atoi(selector)
	path := canonicalPath(selector)
	suffix := strings.TrimSuffix(filepath.Clean(selector), "/")

	var matches []instance
	for _, inst := range instances {
		if inst.PID == pid || inst.Path == path || inst.RemoteHost == selector || hasPathSuffix(inst.Path, suffix) {
			matches = append(matches, inst)
		}
	}
	return matches
}

func hasPathSuffix(path string, suffix string) bool {
	if suffix == "" || suffix == "." || filepath.IsAbs(suffix) {
		return false
	}
	return strings.HasSuffix(path, "/"+suffix)
}

func terminateInstances(instances []instance) error {
	var failures []string
	for _, inst := range instances {
		if err := inst.terminate(); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		log.Printf("stopped process (pid %d) watching %s", inst.PID, color.YellowString(inst.Path))
	}

	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

func init() {
	rootCmd.AddCommand(listCmd, killCmd, killallCmd)
}
