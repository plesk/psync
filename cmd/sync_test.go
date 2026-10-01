// Copyright 1999-2026. WebPros International GmbH.

package cmd

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCollapsePaths(t *testing.T) {
	got := collapsePaths([]string{
		"src/plib/vendor/foo/bar.php",
		"src/plib/vendor/foo",
		"src/plib/vendor/foo",
		"src/plib/vendor/foo-bar/baz.php",
		"src/plib/library/A.php",
	})
	expected := []string{
		"src/plib/library/A.php",
		"src/plib/vendor/foo",
		"src/plib/vendor/foo-bar/baz.php",
	}
	if !slices.Equal(got, expected) {
		t.Errorf("collapsePaths() = %v, expected %v", got, expected)
	}
}

func TestRemoteAndRelativePath(t *testing.T) {
	tests := []struct {
		rule      syncRule
		eventPath string
		remote    string
		relative  string
	}{
		{syncRule{"src/plib", "/usr/local/psa/admin/plib/modules/ext"}, "src/plib/vendor/a.php", "/usr/local/psa/admin/plib/modules/ext/vendor/a.php", "vendor/a.php"},
		{syncRule{"", "/var/www/html"}, "composer.json", "/var/www/html/composer.json", "composer.json"},
	}
	for _, tt := range tests {
		if got := remotePath(tt.rule, tt.eventPath); got != tt.remote {
			t.Errorf("remotePath(%v, %q) = %q, expected %q", tt.rule, tt.eventPath, got, tt.remote)
		}
		if got := relativePath(tt.rule, tt.eventPath); got != tt.relative {
			t.Errorf("relativePath(%v, %q) = %q, expected %q", tt.rule, tt.eventPath, got, tt.relative)
		}
	}
}

func TestWriteTar(t *testing.T) {
	base := t.TempDir()
	mustWrite := func(name string, content string) {
		t.Helper()
		full := filepath.Join(base, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("single.php", "<?php")
	mustWrite("vendor/pkg/lib.php", "lib")
	mustWrite("vendor/pkg/.DS_Store", "junk")
	mustWrite("vendor/pkg/.git/config", "git")
	if err := os.Symlink("../pkg/lib.php", filepath.Join(base, "vendor/link")); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := writeTar(&buf, base, []string{"single.php", "vendor", "vanished.php"}); err != nil {
		t.Fatalf("writeTar() error: %v", err)
	}

	entries := map[string]string{}
	var names []string
	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Uid != 0 || hdr.Gid != 0 {
			t.Errorf("entry %q has owner %d:%d, expected 0:0", hdr.Name, hdr.Uid, hdr.Gid)
		}
		names = append(names, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeReg:
			data, _ := io.ReadAll(tr)
			entries[hdr.Name] = string(data)
		case tar.TypeSymlink:
			entries[hdr.Name] = "-> " + hdr.Linkname
		}
	}

	expectedNames := []string{"single.php", "vendor/", "vendor/link", "vendor/pkg/", "vendor/pkg/lib.php"}
	if !slices.Equal(names, expectedNames) {
		t.Errorf("tar entries = %v, expected %v", names, expectedNames)
	}
	if entries["single.php"] != "<?php" || entries["vendor/pkg/lib.php"] != "lib" {
		t.Errorf("unexpected file contents: %v", entries)
	}
	if entries["vendor/link"] != "-> ../pkg/lib.php" {
		t.Errorf("symlink entry = %q, expected link to ../pkg/lib.php", entries["vendor/link"])
	}
}

func TestSyncQueueCollectsPendingItems(t *testing.T) {
	q := &syncQueue{pending: make(map[syncItem]struct{}), wake: make(chan struct{}, 1)}
	item := syncItem{eventPath: "a.php", sourcePath: "", targetPath: "/var/www"}
	q.add(item)
	q.add(item)
	q.add(syncItem{eventPath: "b.php", sourcePath: "", targetPath: "/var/www"})

	if got := q.take(); len(got) != 2 {
		t.Errorf("take() returned %d items, expected 2 (duplicates collapsed)", len(got))
	}
	if got := q.take(); len(got) != 0 {
		t.Errorf("second take() returned %d items, expected 0", len(got))
	}
}
