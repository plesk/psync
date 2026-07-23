// Copyright 1999-2026. WebPros International GmbH.

package cmd

import (
	"slices"
	"testing"
)

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
