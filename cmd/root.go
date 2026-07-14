// Copyright 1999-2026. WebPros International GmbH.

package cmd

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fsnotify/fsevents"
	"github.com/spf13/cobra"
)

// Version information
var Version string

var pleskMappingRules = map[string]string{
	"common/php/plib":    "/usr/local/psa/admin/plib",
	"common/php/htdocs":  "/usr/local/psa/admin/htdocs",
	"common/application": "/usr/local/psa/admin/application",
}

var pleskExtensionMappingRules = map[string]string{
	"src/plib":   "/usr/local/psa/admin/plib/modules",
	"src/htdocs": "/usr/local/psa/admin/htdocs/modules",
}

var ignorePatterns = []string{"*~", ".*.sw?", "*.tmp", ".DS_Store", "Thumbs.db"}

var currentWorkPath = ""
var remoteHost = ""

var rootCmd = &cobra.Command{
	Use:          "psync",
	Short:        "An utility to sync source code with remote machine",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		remoteHost = getRemoteHost()
		if err := validateRemoteHost(remoteHost); err != nil {
			return err
		}

		return runWatcher()
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
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Start()

	targetFullPath := filepath.Join(targetPath, strings.TrimPrefix(eventPath, sourcePath))
	cmd := exec.Command("scp", "-r", eventPath, remoteHost+":"+targetFullPath)
	err := cmd.Run()
	if err != nil {
		log.Printf("File upload error: %s", err)
	}

	s.Stop()
	log.Printf("Updated %s:%s", remoteHost, targetFullPath)
}

func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func isPleskComposer() bool {
	if !fileExists("composer.json") {
		return false
	}

	data, err := os.ReadFile("composer.json")
	if err != nil {
		return false
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}

	if m["name"] == "plesk/plesk" {
		return true
	}

	return false
}

func getPleskExtensionName(extensionMetaFile string) string {
	f, err := os.Open(extensionMetaFile)
	if err != nil {
		return ""
	}
	defer f.Close()

	dec := xml.NewDecoder(f)
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 && t.Name.Local == "id" {
				var v string
				_ = dec.DecodeElement(&v, &t)
				return v
			}
		case xml.EndElement:
			depth--
		}
	}

	return ""
}

func getMappingRules() map[string]string {
	if isPleskComposer() {
		log.Println("Plesk detected")
		return pleskMappingRules
	}

	extensionMetaFile := filepath.Join("src", "meta.xml")

	if fileExists(extensionMetaFile) {
		extensionName := getPleskExtensionName(extensionMetaFile)
		if extensionName == "" {
			return nil
		}

		for rule, value := range pleskExtensionMappingRules {
			pleskExtensionMappingRules[rule] = filepath.Join(value, extensionName)
		}

		log.Printf("Plesk extension %s detected", extensionName)
		return pleskExtensionMappingRules
	}

	return nil
}

func validateRemoteHost(remoteHost string) error {
	if remoteHost == "" {
		return errors.New("unable to connect: REMOTE_HOST is not set via environment variable or .env file")
	}

	sshArgs := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=5", remoteHost, "true"}
	if err := exec.Command("ssh", sshArgs...).Run(); err != nil {
		return fmt.Errorf("SSH connection test failed: %w", err)
	}

	return nil
}

func validateProductPresence(mappingRules map[string]string) error {
	if mappingRules == nil {
		return errors.New("unknown source tree")
	}

	for _, dir := range mappingRules {
		sshArgs := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=5", remoteHost, "test", "-d", dir}
		if err := exec.Command("ssh", sshArgs...).Run(); err != nil {
			return fmt.Errorf("remote directory %q does not exist on %s: %w", dir, remoteHost, err)
		}
		break
	}

	return nil
}

func getRemoteHost() string {
	if host := os.Getenv("REMOTE_HOST"); host != "" {
		return host
	}

	return getEnvFileValue(".env", "REMOTE_HOST")
}

func getEnvFileValue(envFile string, key string) string {
	data, err := os.ReadFile(envFile)
	if err != nil {
		return ""
	}

	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		name, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "export "))
		if name != key {
			continue
		}

		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}

		return value
	}

	return ""
}

func runWatcher() error {
	mappingRules := getMappingRules()
	if err := validateProductPresence(mappingRules); err != nil {
		return err
	}

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

	debounce := newDebouncer(300 * time.Millisecond)

	for msg := range es.Events {
		for _, e := range msg {
			eventPath := trimPath(e.Path)

			for sourcePath, targetPath := range mappingRules {
				if !strings.HasPrefix(eventPath, sourcePath) || isIgnored(eventPath) {
					continue
				}

				if e.Flags&fsevents.ItemModified != 0 || e.Flags&fsevents.ItemInodeMetaMod != 0 {
					debounce.trigger(eventPath, func() {
						uploadFile(eventPath, sourcePath, targetPath)
					})
				}
			}
		}
	}

	return nil
}

func init() {
	currentWorkPath, _ = os.Getwd()
}

func Execute() {
	rootCmd.Version = Version
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.Flags().BoolP("version", "v", false, "Print version information")

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
