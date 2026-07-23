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

type changeset struct {
	uploads  []string
	removals []string
}

func getChangedFiles() (changeset, error) {
	out, err := exec.Command("git", "status", "--porcelain", "-z").Output()
	if err != nil {
		return changeset{}, fmt.Errorf("git status failed: %w", err)
	}

	return parseGitStatus(string(out)), nil
}

func parseGitStatus(out string) changeset {
	var changes changeset

	entries := strings.Split(out, "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}

		status, file := entry[:2], entry[3:]
		if status[0] == 'R' || status[0] == 'C' {
			i++
			if status[0] == 'R' && i < len(entries) {
				changes.removals = append(changes.removals, entries[i])
			}
		}
		if status[0] == 'D' || status[1] == 'D' {
			changes.removals = append(changes.removals, file)
			continue
		}

		changes.uploads = append(changes.uploads, file)
	}

	return changes
}

func runDiff() error {
	mappingRules := getMappingRules()
	if err := validateProductPresence(mappingRules); err != nil {
		return err
	}

	changes, err := getChangedFiles()
	if err != nil {
		return err
	}

	processed := processFiles(changes.uploads, mappingRules, uploadFile)
	processed += processFiles(changes.removals, mappingRules, removeFile)

	if processed == 0 {
		log.Println("no changed files to upload")
	}

	return nil
}

func processFiles(files []string, mappingRules map[string]string, action func(file string, sourcePath string, targetPath string)) int {
	processed := 0
	for _, file := range files {
		if isIgnored(file) {
			continue
		}

		for sourcePath, targetPath := range mappingRules {
			if strings.HasPrefix(file, sourcePath+"/") {
				action(file, sourcePath, targetPath)
				processed++
			}
		}
	}

	return processed
}

func init() {
	rootCmd.AddCommand(diffCmd)
}
