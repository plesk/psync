// Copyright 1999-2026. WebPros International GmbH.

package cmd

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff [ref]",
	Short: "Upload files changed according to git status and exit",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := changeWorkDir(workDirFlag); err != nil {
			return err
		}

		remoteHost = getRemoteHost()
		if err := validateRemoteHost(remoteHost); err != nil {
			return err
		}

		base := ""
		if len(args) > 0 {
			base = args[0]
		}

		return runDiff(base)
	},
}

type changeset struct {
	uploads  []string
	removals []string
}

func getChangedFiles(base string) (changeset, error) {
	if base != "" {
		return getChangedFilesForRef(base)
	}

	out, err := exec.Command("git", "status", "--porcelain", "-z").Output()
	if err != nil {
		return changeset{}, fmt.Errorf("git status failed: %w", err)
	}

	return parseGitStatus(string(out)), nil
}

func getChangedFilesForRef(ref string) (changeset, error) {
	var gitArgs []string
	if strings.Contains(ref, "..") {
		gitArgs = []string{"diff", "--name-status", "-z", ref, "--"}
	} else {
		gitArgs = []string{"show", "--first-parent", "--name-status", "--format=", "-z", ref, "--"}
	}

	out, err := exec.Command("git", gitArgs...).Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && len(exitErr.Stderr) > 0 {
			return changeset{}, fmt.Errorf("git %s failed: %s", gitArgs[0], strings.TrimSpace(string(exitErr.Stderr)))
		}
		return changeset{}, fmt.Errorf("git %s failed: %w", gitArgs[0], err)
	}

	return parseGitDiffNameStatus(string(out)), nil
}

func parseGitDiffNameStatus(out string) changeset {
	var changes changeset

	entries := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	for i := 0; i+1 < len(entries); i += 2 {
		status, file := entries[i], entries[i+1]
		if status == "" {
			continue
		}

		switch status[0] {
		case 'R', 'C':
			if i+2 >= len(entries) {
				return changes
			}
			if status[0] == 'R' {
				changes.removals = append(changes.removals, file)
			}
			changes.uploads = append(changes.uploads, entries[i+2])
			i++
		case 'D':
			changes.removals = append(changes.removals, file)
		default:
			changes.uploads = append(changes.uploads, file)
		}
	}

	return changes
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

func runDiff(base string) error {
	mappingRules := getMappingRules()
	if err := validateProductPresence(mappingRules); err != nil {
		return err
	}

	changes, err := getChangedFiles(base)
	if err != nil {
		return err
	}

	var items []syncItem
	add := func(file string, sourcePath string, targetPath string) {
		items = append(items, syncItem{eventPath: file, sourcePath: sourcePath, targetPath: targetPath})
	}
	processed := processFiles(changes.uploads, mappingRules, add)
	processed += processFiles(changes.removals, mappingRules, add)

	if processed == 0 {
		log.Println("no changed files to upload")
		return nil
	}

	syncItems(items)
	return nil
}

func processFiles(files []string, mappingRules map[string]string, action func(file string, sourcePath string, targetPath string)) int {
	processed := 0
	for _, file := range files {
		if isIgnored(file) {
			continue
		}

		for sourcePath, targetPath := range mappingRules {
			if sourcePath == "" || strings.HasPrefix(file, sourcePath+"/") {
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
