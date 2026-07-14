package cmd

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
)

func TestIsIgnored(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"src/plib/library/Utils.php", false},
		{"src/plib/library/Utils.php~", true},
		{"src/plib/.Utils.php.swp", true},
		{"src/plib/.Utils.php.swx", true},
		{"src/plib/library/cache.tmp", true},
		{".DS_Store", true},
		{"src/htdocs/.DS_Store", true},
		{"Thumbs.db", true},
		{"src/htdocs/images/Thumbs.db", true},
		{"composer.json", false},
		{"src/plib/tmp.php", false},
	}

	for _, tt := range tests {
		if got := isIgnored(tt.path); got != tt.expected {
			t.Errorf("isIgnored(%q) = %v, expected %v", tt.path, got, tt.expected)
		}
	}
}

func TestTrimPath(t *testing.T) {
	origWorkPath := currentWorkPath
	defer func() { currentWorkPath = origWorkPath }()

	currentWorkPath = "/Users/john/project"

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"inside work path", "/Users/john/project/src/plib/file.php", "src/plib/file.php"},
		{"work path itself", "/Users/john/project", "/Users/john/project"},
		{"outside work path", "/etc/hosts", "/etc/hosts"},
		{"relative path is rooted first", "/Users/john/project/src/file.php", "src/file.php"},
		{"path with dot segments", "/Users/john/project/src/../src/file.php", "src/file.php"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimPath(tt.path); got != tt.expected {
				t.Errorf("trimPath(%q) = %q, expected %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()

	existing := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(existing, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	if !fileExists(existing) {
		t.Errorf("fileExists(%q) = false, expected true", existing)
	}

	if !fileExists(dir) {
		t.Errorf("fileExists(%q) = false for a directory, expected true", dir)
	}

	missing := filepath.Join(dir, "missing.txt")
	if fileExists(missing) {
		t.Errorf("fileExists(%q) = true, expected false", missing)
	}
}

func TestIsPleskComposer(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		create   bool
		expected bool
	}{
		{"plesk composer", `{"name": "plesk/plesk"}`, true, true},
		{"other composer", `{"name": "vendor/package"}`, true, false},
		{"no name field", `{"description": "something"}`, true, false},
		{"invalid json", `{not json`, true, false},
		{"no composer.json", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if tt.create {
				if err := os.WriteFile("composer.json", []byte(tt.content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			if got := isPleskComposer(); got != tt.expected {
				t.Errorf("isPleskComposer() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestGetPleskExtensionName(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			"valid meta.xml",
			`<?xml version="1.0"?><module><id>my-extension</id><name>My Extension</name></module>`,
			"my-extension",
		},
		{
			"id is not the first element",
			`<?xml version="1.0"?><module><name>My Extension</name><id>my-extension</id></module>`,
			"my-extension",
		},
		{
			"no id element",
			`<?xml version="1.0"?><module><name>My Extension</name></module>`,
			"",
		},
		{
			"malformed xml with unclosed id",
			`<module><id>my-extension`,
			"",
		},
		{
			"empty file",
			``,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metaFile := filepath.Join(t.TempDir(), "meta.xml")
			if err := os.WriteFile(metaFile, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			if got := getPleskExtensionName(metaFile); got != tt.expected {
				t.Errorf("getPleskExtensionName() = %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestGetPleskExtensionNameMissingFile(t *testing.T) {
	if got := getPleskExtensionName(filepath.Join(t.TempDir(), "meta.xml")); got != "" {
		t.Errorf("getPleskExtensionName() = %q for a missing file, expected empty string", got)
	}
}

func TestGetMappingRulesPlesk(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := os.WriteFile("composer.json", []byte(`{"name": "plesk/plesk"}`), 0644); err != nil {
		t.Fatal(err)
	}

	rules := getMappingRules()
	if !maps.Equal(rules, pleskMappingRules) {
		t.Errorf("getMappingRules() = %v, expected plesk mapping rules %v", rules, pleskMappingRules)
	}
}

func TestValidateRemoteHostEmpty(t *testing.T) {
	err := validateRemoteHost("")
	if err == nil {
		t.Fatal("validateRemoteHost(\"\") = nil, expected an error")
	}
}

func TestValidateProductPresenceNilRules(t *testing.T) {
	err := validateProductPresence(nil)
	if err == nil {
		t.Fatal("validateProductPresence(nil) = nil, expected an error")
	}
}
