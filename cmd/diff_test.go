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
		expected []string
	}{
		{
			"empty output",
			"",
			nil,
		},
		{
			"modified file",
			" M src/plib/library/Utils.php\x00",
			[]string{"src/plib/library/Utils.php"},
		},
		{
			"staged and untracked files",
			"A  src/plib/library/New.php\x00?? src/htdocs/index.php\x00",
			[]string{"src/plib/library/New.php", "src/htdocs/index.php"},
		},
		{
			"deleted files are skipped",
			" D src/plib/library/Gone.php\x00D  src/plib/library/Staged.php\x00 M src/plib/library/Kept.php\x00",
			[]string{"src/plib/library/Kept.php"},
		},
		{
			"renamed file keeps new path only",
			"R  src/plib/library/New.php\x00src/plib/library/Old.php\x00 M src/htdocs/index.php\x00",
			[]string{"src/plib/library/New.php", "src/htdocs/index.php"},
		},
		{
			"file name with spaces",
			" M src/htdocs/my file.php\x00",
			[]string{"src/htdocs/my file.php"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseGitStatus(tt.out); !slices.Equal(got, tt.expected) {
				t.Errorf("parseGitStatus(%q) = %v, expected %v", tt.out, got, tt.expected)
			}
		})
	}
}
