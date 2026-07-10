package cmd

import (
	"encoding/json"
	"encoding/xml"
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

func runWatcher() {
	mappingRules := getMappingRules()

	if mappingRules == nil {
		log.Println("Unknown source tree, exiting...")
		return
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
