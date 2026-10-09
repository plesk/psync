// Copyright 1999-2026. WebPros International GmbH.

package cmd

import (
	"slices"
	"strings"
	"testing"
)

func TestProcessFilesGenericMapping(t *testing.T) {
	var processed []string
	count := processFiles(
		[]string{"composer.json", "src/lib/file.php"},
		map[string]string{"": "/var/www/html"},
		func(file string, sourcePath string, targetPath string) {
			processed = append(processed, file+" -> "+targetPath)
		},
	)

	expected := []string{
		"composer.json -> /var/www/html",
		"src/lib/file.php -> /var/www/html",
	}
	if count != len(expected) || !slices.Equal(processed, expected) {
		t.Errorf("processFiles() = (%d, %v), expected (%d, %v)", count, processed, len(expected), expected)
	}
}

func TestParseGitStatus(t *testing.T) {
	tests := []struct {
		name     string
		out      string
		uploads  []string
		removals []string
	}{
		{
			"empty output",
			"",
			nil,
			nil,
		},
		{
			"modified file",
			" M src/plib/library/Utils.php\x00",
			[]string{"src/plib/library/Utils.php"},
			nil,
		},
		{
			"staged and untracked files",
			"A  src/plib/library/New.php\x00?? src/htdocs/index.php\x00",
			[]string{"src/plib/library/New.php", "src/htdocs/index.php"},
			nil,
		},
		{
			"deleted files are removed",
			" D src/plib/library/Gone.php\x00D  src/plib/library/Staged.php\x00 M src/plib/library/Kept.php\x00",
			[]string{"src/plib/library/Kept.php"},
			[]string{"src/plib/library/Gone.php", "src/plib/library/Staged.php"},
		},
		{
			"renamed file uploads new path and removes old one",
			"R  src/plib/library/New.php\x00src/plib/library/Old.php\x00 M src/htdocs/index.php\x00",
			[]string{"src/plib/library/New.php", "src/htdocs/index.php"},
			[]string{"src/plib/library/Old.php"},
		},
		{
			"copied file keeps original",
			"C  src/plib/library/Copy.php\x00src/plib/library/Original.php\x00",
			[]string{"src/plib/library/Copy.php"},
			nil,
		},
		{
			"renamed then deleted file removes both paths",
			"RD src/plib/library/New.php\x00src/plib/library/Old.php\x00",
			nil,
			[]string{"src/plib/library/Old.php", "src/plib/library/New.php"},
		},
		{
			"file name with spaces",
			" M src/htdocs/my file.php\x00",
			[]string{"src/htdocs/my file.php"},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGitStatus(tt.out)
			if !slices.Equal(got.uploads, tt.uploads) {
				t.Errorf("parseGitStatus(%q) uploads = %v, expected %v", tt.out, got.uploads, tt.uploads)
			}
			if !slices.Equal(got.removals, tt.removals) {
				t.Errorf("parseGitStatus(%q) removals = %v, expected %v", tt.out, got.removals, tt.removals)
			}
		})
	}
}

func TestParseGitDiffNameStatus(t *testing.T) {
	tests := []struct {
		name     string
		out      string
		uploads  []string
		removals []string
	}{
		{
			"empty output",
			"",
			nil,
			nil,
		},
		{
			"modified and added files",
			"M\x00src/plib/library/Utils.php\x00A\x00src/htdocs/index.php\x00",
			[]string{"src/plib/library/Utils.php", "src/htdocs/index.php"},
			nil,
		},
		{
			"deleted file is removed",
			"D\x00src/plib/library/Gone.php\x00M\x00src/plib/library/Kept.php\x00",
			[]string{"src/plib/library/Kept.php"},
			[]string{"src/plib/library/Gone.php"},
		},
		{
			"renamed file uploads new path and removes old one",
			"R100\x00src/plib/library/Old.php\x00src/plib/library/New.php\x00M\x00src/htdocs/index.php\x00",
			[]string{"src/plib/library/New.php", "src/htdocs/index.php"},
			[]string{"src/plib/library/Old.php"},
		},
		{
			"copied file keeps original",
			"C075\x00src/plib/library/Original.php\x00src/plib/library/Copy.php\x00",
			[]string{"src/plib/library/Copy.php"},
			nil,
		},
		{
			"type change is uploaded",
			"T\x00src/htdocs/link.php\x00",
			[]string{"src/htdocs/link.php"},
			nil,
		},
		{
			"file name with spaces",
			"M\x00src/htdocs/my file.php\x00",
			[]string{"src/htdocs/my file.php"},
			nil,
		},
		{
			"truncated rename entry is ignored",
			"R100\x00src/plib/library/Old.php\x00",
			nil,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGitDiffNameStatus(tt.out)
			if !slices.Equal(got.uploads, tt.uploads) {
				t.Errorf("parseGitDiffNameStatus(%q) uploads = %v, expected %v", tt.out, got.uploads, tt.uploads)
			}
			if !slices.Equal(got.removals, tt.removals) {
				t.Errorf("parseGitDiffNameStatus(%q) removals = %v, expected %v", tt.out, got.removals, tt.removals)
			}
		})
	}
}

func TestGetChangedFilesForRefUnknownRef(t *testing.T) {
	_, err := getChangedFilesForRef("psync-nonexistent-ref")
	if err == nil {
		t.Fatal("getChangedFilesForRef() expected an error for unknown ref")
	}
	if !strings.Contains(err.Error(), "git show failed") || !strings.Contains(err.Error(), "psync-nonexistent-ref") {
		t.Errorf("getChangedFilesForRef() error = %q, expected git stderr to be included", err)
	}

	_, err = getChangedFilesForRef("psync-nonexistent-ref..HEAD")
	if err == nil {
		t.Fatal("getChangedFilesForRef() expected an error for unknown range")
	}
	if !strings.Contains(err.Error(), "git diff failed") || !strings.Contains(err.Error(), "psync-nonexistent-ref") {
		t.Errorf("getChangedFilesForRef() error = %q, expected git stderr to be included", err)
	}
}
