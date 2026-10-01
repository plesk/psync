// Copyright 1999-2026. WebPros International GmbH.

package cmd

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	"plib":   "/usr/local/psa/admin/plib/modules/<extension-id>",
	"htdocs": "/usr/local/psa/admin/htdocs/modules/<extension-id>",
	"sbin":   "/usr/local/psa/admin/sbin/modules/<extension-id>",
	"_meta":  "/usr/local/psa/admin/share/modules/<extension-id>/_meta",
}

var ignorePatterns = []string{"*~", ".*.sw?", "*.tmp", "*.tmp.*", ".DS_Store", "Thumbs.db"}

var currentWorkPath = ""
var remoteHost = ""
var workDirFlag = ""
var destinationFlag = ""

type timestampWriter struct {
	w io.Writer
}

func (t timestampWriter) Write(p []byte) (int, error) {
	if _, err := io.WriteString(t.w, time.Now().Format("15:04:05.000 ")); err != nil {
		return 0, err
	}
	return t.w.Write(p)
}

func setupLogging() {
	log.SetFlags(0)
	log.SetOutput(timestampWriter{w: os.Stderr})
}

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

func sshCommand(name string, args ...string) *exec.Cmd {
	controlDir := "/tmp"
	if home, err := os.UserHomeDir(); err == nil {
		controlDir = filepath.Join(home, ".ssh")
	}
	controlPath := filepath.Join(controlDir, "psync-%C")
	muxArgs := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + controlPath,
		"-o", "ControlPersist=60",
	}
	return exec.Command(name, append(muxArgs, args...)...)
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

func dirExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.IsDir()
}

func getPleskExtensionMappingRules(extensionName string, hasSrcDir bool) map[string]string {
	rules := make(map[string]string, len(pleskExtensionMappingRules)+1)
	for rule, value := range pleskExtensionMappingRules {
		value = strings.ReplaceAll(value, "<extension-id>", extensionName)
		if hasSrcDir {
			rules["src/"+rule] = value
		}
		if !hasSrcDir || rule == "_meta" {
			rules[rule] = value
		}
	}
	return rules
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

		rules := getPleskExtensionMappingRules(extensionName, dirExists("src"))

		log.Printf("%s %s %s",
			color.CyanString("Plesk extension"),
			color.New(color.FgHiCyan, color.Bold).Sprint(extensionName),
			color.CyanString("detected"))
		return rules
	}

	return nil
}

func formatMappingRules(mappingRules map[string]string) string {
	locals := make([]string, 0, len(mappingRules))
	width := 0
	for local := range mappingRules {
		locals = append(locals, local)
		if l := len(displayLocalPath(local)); l > width {
			width = l
		}
	}
	sort.Strings(locals)

	var b strings.Builder
	for _, local := range locals {
		_, _ = fmt.Fprintf(&b, "  %-*s -> %s\n", width, displayLocalPath(local), mappingRules[local])
	}
	return strings.TrimRight(b.String(), "\n")
}

func displayLocalPath(local string) string {
	if local == "" {
		return "."
	}
	return local
}

func printMappingRules(mappingRules map[string]string) {
	if len(mappingRules) == 0 {
		return
	}
	log.Printf("%s\n%s", color.CyanString("mapping (local -> %s):", remoteHost), formatMappingRules(mappingRules))
}

func validateRemoteHost(remoteHost string) error {
	if remoteHost == "" {
		return errors.New("unable to connect: REMOTE_HOST is not set via environment variable or .env file")
	}

	if strings.ContainsAny(remoteHost, " \t") {
		return fmt.Errorf("invalid remote host %q: it must not contain spaces", remoteHost)
	}

	sshArgs := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=5", remoteHost, "true"}
	if err := sshCommand("ssh", sshArgs...).Run(); err != nil {
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
		if err := sshCommand("ssh", sshArgs...).Run(); err != nil {
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
	printMappingRules(mappingRules)

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
	queue := newSyncQueue()

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
						queue.add(syncItem{eventPath: eventPath, sourcePath: sourcePath, targetPath: targetPath})
					})
				}
			}
		}
	}

	return nil
}

func init() {
	currentWorkPath, _ = os.Getwd()
	setupLogging()
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
