// Copyright 1999-2026. WebPros International GmbH.

package cmd

import (
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Upload files changed according to git status and exit",
	RunE: func(cmd *cobra.Command, args []string) error {
		remoteHost = getRemoteHost()
		if err := validateRemoteHost(remoteHost); err != nil {
			return err
		}

		return runDiff()
	},
}

func getChangedFiles() ([]string, error) {
	out, err := exec.Command("git", "status", "--porcelain", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	return parseGitStatus(string(out)), nil
}

func parseGitStatus(out string) []string {
	var files []string

	entries := strings.Split(out, "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}

		status, file := entry[:2], entry[3:]
		if status[0] == 'R' || status[0] == 'C' {
			i++
		}
		if status[0] == 'D' || status[1] == 'D' {
			continue
		}

		files = append(files, file)
	}

	return files
}

func runDiff() error {
	mappingRules := getMappingRules()
	if err := validateProductPresence(mappingRules); err != nil {
		return err
	}

	files, err := getChangedFiles()
	if err != nil {
		return err
	}

	uploaded := 0
	for _, file := range files {
		if isIgnored(file) {
			continue
		}

		for sourcePath, targetPath := range mappingRules {
			if strings.HasPrefix(file, sourcePath+"/") {
				uploadFile(file, sourcePath, targetPath)
				uploaded++
			}
		}
	}

	if uploaded == 0 {
		log.Println("no changed files to upload")
	}

	return nil
}

func init() {
	rootCmd.AddCommand(diffCmd)
}
