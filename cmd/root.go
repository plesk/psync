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
	"github.com/fatih/color"
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
	"src/plib":   "/usr/local/psa/admin/plib/modules/<extension-id>",
	"src/htdocs": "/usr/local/psa/admin/htdocs/modules/<extension-id>",
	"src/sbin":   "/usr/local/psa/admin/sbin/modules/<extension-id>",
	"src/_meta":  "/usr/local/psa/admin/share/modules/<extension-id>/_meta",
	"_meta":      "/usr/local/psa/admin/share/modules/<extension-id>/_meta",
}

var ignorePatterns = []string{"*~", ".*.sw?", "*.tmp", "*.tmp.*", ".DS_Store", "Thumbs.db"}

var currentWorkPath = ""
var remoteHost = ""
var workDirFlag = ""
var destinationFlag = ""

var rootCmd = &cobra.Command{
	Use:          "psync",
	Short:        "A utility to sync source code with a remote machine",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := changeWorkDir(workDirFlag); err != nil {
			return err
		}

		remoteHost = getRemoteHost()
		if err := validateRemoteHost(remoteHost); err != nil {
			return err
		}

		return runWatcher()
	},
}

func isIgnored(eventPath string) bool {
	for _, part := range strings.Split(filepath.ToSlash(eventPath), "/") {
		if len(part) > 1 && part[0] == '.' && part != ".." {
			return true
		}
	}

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

func upload(eventPath string, sourcePath string, targetPath string) {
	if isDirectory(eventPath) {
		uploadDirectory(eventPath, sourcePath, targetPath)
	} else {
		uploadFile(eventPath, sourcePath, targetPath)
	}
}

func uploadFile(eventPath string, sourcePath string, targetPath string) {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Start()

	targetFullPath := filepath.Join(targetPath, strings.TrimPrefix(eventPath, sourcePath))
	out, err := exec.Command("scp", eventPath, remoteHost+":"+targetFullPath).CombinedOutput()

	s.Stop()
	if err != nil {
		log.Print(color.RedString("file upload error: %s%s", err, formatCommandOutput(out)))
		return
	}
	log.Printf("updated %s", color.GreenString("%s:%s", remoteHost, targetFullPath))
}

func uploadDirectory(eventPath string, sourcePath string, targetPath string) {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Start()

	targetFullPath := filepath.Join(targetPath, strings.TrimPrefix(eventPath, sourcePath))
	out, err := exec.Command("ssh", remoteHost, "mkdir", "-p", fmt.Sprintf("%q", targetFullPath)).CombinedOutput()
	if err == nil {
		scpTarget := remoteHost + ":" + filepath.Dir(targetFullPath)
		out, err = exec.Command("scp", "-r", filepath.Clean(eventPath), scpTarget).CombinedOutput()
	}

	s.Stop()
	if err != nil {
		log.Print(color.RedString("directory upload error: %s%s", err, formatCommandOutput(out)))
		return
	}
	log.Printf("updated %s", color.GreenString("%s:%s", remoteHost, targetFullPath))
}

func removePath(eventPath string, sourcePath string, targetPath string) {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Start()

	targetFullPath := filepath.Join(targetPath, strings.TrimPrefix(eventPath, sourcePath))
	out, err := exec.Command("ssh", remoteHost, "rm", "-rf", fmt.Sprintf("%q", targetFullPath)).CombinedOutput()

	s.Stop()
	if err != nil {
		log.Print(color.RedString("removal error: %s%s", err, formatCommandOutput(out)))
		return
	}
	log.Printf("removed %s", color.YellowString("%s:%s", remoteHost, targetFullPath))
}

func formatCommandOutput(out []byte) string {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return ""
	}
	return ": " + trimmed
}

func isDirectory(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.IsDir()
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
	if destinationFlag != "" {
		return map[string]string{"": destinationFlag}
	}

	if isPleskComposer() {
		log.Print(color.CyanString("Plesk detected"))
		return pleskMappingRules
	}

	extensionMetaFile := filepath.Join("src", "meta.xml")
	if !fileExists(extensionMetaFile) {
		extensionMetaFile = "meta.xml"
	}

	if fileExists(extensionMetaFile) {
		extensionName := getPleskExtensionName(extensionMetaFile)
		if extensionName == "" {
			return nil
		}

		rules := make(map[string]string, len(pleskExtensionMappingRules))
		for rule, value := range pleskExtensionMappingRules {
			rules[rule] = strings.ReplaceAll(value, "<extension-id>", extensionName)
		}

		log.Printf("%s %s %s",
			color.CyanString("Plesk extension"),
			color.New(color.FgHiCyan, color.Bold).Sprint(extensionName),
			color.CyanString("detected"))
		return rules
	}

	return nil
}

func validateRemoteHost(remoteHost string) error {
	if remoteHost == "" {
		return errors.New("unable to connect: REMOTE_HOST is not set via environment variable or .env file")
	}

	if strings.ContainsAny(remoteHost, " \t") {
		return fmt.Errorf("invalid remote host %q: it must not contain spaces", remoteHost)
	}

	sshArgs := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=5", remoteHost, "true"}
	if err := exec.Command("ssh", sshArgs...).Run(); err != nil {
		return fmt.Errorf("unable to connect to %s non-interactively: %w; "+
			"set up SSH key-based authentication (e.g. ssh-copy-id %s)", remoteHost, err, remoteHost)
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

func changeWorkDir(dir string) error {
	if dir == "" {
		return nil
	}

	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("unable to change the monitoring directory: %w", err)
	}

	currentWorkPath, _ = os.Getwd()
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

	log.Print(color.New(color.FgGreen, color.Bold).Sprint("watcher is ready..."))

	debounce := newDebouncer(300 * time.Millisecond)

	for msg := range es.Events {
		for _, e := range msg {
			eventPath := trimPath(e.Path)

			for sourcePath, targetPath := range mappingRules {
				if !strings.HasPrefix(eventPath, sourcePath) || isIgnored(eventPath) {
					continue
				}

				const changeFlags = fsevents.ItemModified | fsevents.ItemInodeMetaMod |
					fsevents.ItemRenamed | fsevents.ItemCreated | fsevents.ItemRemoved
				if e.Flags&changeFlags != 0 {
					debounce.trigger(eventPath, func() {
						if fileExists(eventPath) {
							upload(eventPath, sourcePath, targetPath)
						} else {
							removePath(eventPath, sourcePath, targetPath)
						}
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
	rootCmd.Flags().StringVarP(&workDirFlag, "chdir", "c", "", "Directory to monitor (defaults to the current one)")
	rootCmd.Flags().StringVarP(&destinationFlag, "destination", "d", "", "Remote directory to sync the current one to")

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
