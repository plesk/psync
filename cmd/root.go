package cmd

import (
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsevents"
	"github.com/spf13/cobra"
)

var mappingRules = map[string]string{
	"common/php/plib":    "/usr/local/psa/admin/plib",
	"common/php/htdocs":  "/usr/local/psa/admin/htdocs",
	"common/application": "/usr/local/psa/admin/application",
}

var ignorePatterns = []string{"*~", ".*.sw?", "*.tmp", ".DS_Store", "Thumbs.db"}

var currentWorkPath = ""
var remoteHost = ""

var rootCmd = &cobra.Command{
	Use:   "psync",
	Short: "An utility to sync source code with remote machine",
	RunE: func(cmd *cobra.Command, args []string) error {
		runWatcher()
		return nil
	},
}

func isIgnored(eventPath string) bool {
	fileName := filepath.Base(eventPath)

	for _, pattern := range ignorePatterns {
		if ok, _ := path.Match(pattern, fileName); ok {
			return true
		}
	}

	return false
}

func trimPath(targetPath string) string {
	targetPath = filepath.Join("/", targetPath)
	targetPath = strings.TrimPrefix(targetPath, currentWorkPath+"/")
	return targetPath
}

func uploadFile(eventPath string, sourcePath string, targetPath string) {
	targetFullPath := filepath.Join(targetPath, strings.TrimPrefix(eventPath, sourcePath))
	cmd := exec.Command("scp", "-r", eventPath, remoteHost+":"+targetFullPath)
	err := cmd.Run()
	if err != nil {
		log.Printf("File upload error: %s", err)
	}

	log.Printf("Uploaded file %s -> %s:%s", eventPath, remoteHost, targetFullPath)
}

func runWatcher() {
	dev, _ := fsevents.DeviceForPath(".")
	es := &fsevents.EventStream{
		Paths:   []string{"."},
		Latency: 500 * time.Millisecond,
		Device:  dev,
		Flags:   fsevents.FileEvents | fsevents.WatchRoot,
	}
	_ = es.Start()
	defer es.Stop()

	log.Printf("Watcher is ready...")

	for msg := range es.Events {
		for _, e := range msg {
			eventPath := trimPath(e.Path)

			for sourcePath, targetPath := range mappingRules {
				if !strings.HasPrefix(eventPath, sourcePath) || isIgnored(eventPath) {
					continue
				}

				if e.Flags&fsevents.ItemModified != 0 || e.Flags&fsevents.ItemInodeMetaMod != 0 {
					uploadFile(eventPath, sourcePath, targetPath)
				}
			}
		}
	}
}

func init() {
	currentWorkPath, _ = os.Getwd()
	remoteHost = os.Getenv("REMOTE_HOST")

	rootCmd.CompletionOptions.DisableDefaultCmd = true

	rootCmd.AddCommand(
		versionCmd,
	)
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
