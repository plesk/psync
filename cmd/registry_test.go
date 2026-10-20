// Copyright 1999-2026. WebPros International GmbH.

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func setupRegistry(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return filepath.Join(home, ".psync")
}

func writeInstance(t *testing.T, dir string, name string, inst instance) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(inst)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, name)
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestRegisterInstanceRoundTrip(t *testing.T) {
	dir := setupRegistry(t)
	watchPath := t.TempDir()

	unregister, err := registerInstance(watchPath, "root@example.com")
	if err != nil {
		t.Fatalf("registerInstance: %v", err)
	}

	file := instanceFile(dir, os.Getpid())
	if !fileExists(file) {
		t.Fatalf("registry file %s was not created", file)
	}

	instances, err := loadInstances()
	if err != nil {
		t.Fatalf("loadInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("loadInstances returned %d instances, expected 1", len(instances))
	}

	inst := instances[0]
	if inst.PID != os.Getpid() {
		t.Errorf("pid = %d, expected %d", inst.PID, os.Getpid())
	}
	if inst.Path != canonicalPath(watchPath) {
		t.Errorf("path = %q, expected %q", inst.Path, canonicalPath(watchPath))
	}
	if inst.RemoteHost != "root@example.com" {
		t.Errorf("remote host = %q, expected root@example.com", inst.RemoteHost)
	}

	unregister()
	if fileExists(file) {
		t.Errorf("registry file %s still exists after unregister", file)
	}
}

func TestLoadInstancesEmpty(t *testing.T) {
	setupRegistry(t)

	instances, err := loadInstances()
	if err != nil {
		t.Fatalf("loadInstances: %v", err)
	}
	if len(instances) != 0 {
		t.Errorf("expected no instances, got %d", len(instances))
	}
}

func TestInstanceIsAlive(t *testing.T) {
	tests := []struct {
		name     string
		inst     instance
		expected bool
	}{
		{"self", instance{PID: os.Getpid()}, true},
		{"dead pid", instance{PID: 999999}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.inst.isAlive(); got != tt.expected {
				t.Errorf("isAlive() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestCanonicalPath(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	expected := canonicalPath(real)
	if got := canonicalPath(link); got != expected {
		t.Errorf("canonicalPath(symlink) = %q, expected %q", got, expected)
	}
	if got := canonicalPath(real + "/"); got != expected {
		t.Errorf("canonicalPath(trailing slash) = %q, expected %q", got, expected)
	}

	missing := filepath.Join(dir, "missing")
	if got := canonicalPath(missing); !filepath.IsAbs(got) {
		t.Errorf("canonicalPath(missing) = %q, expected an absolute path", got)
	}
}

func TestFindInstances(t *testing.T) {
	dir := t.TempDir()
	watched := filepath.Join(dir, "extensions", "ext-foo")
	if err := os.MkdirAll(watched, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(watched, link); err != nil {
		t.Fatal(err)
	}

	instances := []instance{
		{PID: 101, Path: canonicalPath(watched), RemoteHost: "root@10.0.0.1"},
		{PID: 102, Path: canonicalPath(watched), RemoteHost: "devbox"},
		{PID: 103, Path: canonicalPath(dir) + "/plesk", RemoteHost: "devbox"},
		{PID: 104, Path: "/srv/ext-foo", RemoteHost: "other"},
	}

	t.Chdir(dir)

	tests := []struct {
		name     string
		selector string
		expected []int
	}{
		{"pid", "102", []int{102}},
		{"unknown pid", "999", nil},
		{"remote host", "devbox", []int{102, 103}},
		{"exact path", watched, []int{101, 102}},
		{"symlinked path", link, []int{101, 102}},
		{"trailing slash", watched + "/", []int{101, 102}},
		{"relative path", "extensions/ext-foo", []int{101, 102}},
		{"last component suffix", "ext-foo", []int{101, 102, 104}},
		{"multi-component suffix", "srv/ext-foo", []int{104}},
		{"suffix with trailing slash", "srv/ext-foo/", []int{104}},
		{"partial component is not a suffix", "foo", nil},
		{"absolute selector is not a suffix", "/ext-foo", nil},
		{"unknown", "nothing", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int
			for _, inst := range findInstances(instances, tt.selector) {
				got = append(got, inst.PID)
			}
			if !slices.Equal(got, tt.expected) {
				t.Errorf("findInstances(%q) = %v, expected %v", tt.selector, got, tt.expected)
			}
		})
	}
}

func TestPrintInstances(t *testing.T) {
	var buf bytes.Buffer
	err := printInstances(&buf, []instance{
		{PID: 42, Path: "/Users/john/ext-a", RemoteHost: "root@10.0.0.1"},
		{PID: 4242, Path: "/Users/john/plesk", RemoteHost: "plesk-dev"},
	})
	if err != nil {
		t.Fatal(err)
	}

	expected := "PID   PATH               REMOTE HOST\n" +
		"42    /Users/john/ext-a  root@10.0.0.1\n" +
		"4242  /Users/john/plesk  plesk-dev\n"
	if buf.String() != expected {
		t.Errorf("unexpected output:\n%s\nexpected:\n%s", buf.String(), expected)
	}
}

func TestPrintInstancesEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := printInstances(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "PID  PATH  REMOTE HOST\n" {
		t.Errorf("unexpected output: %q", buf.String())
	}
}

func TestTerminateInstance(t *testing.T) {
	dir := setupRegistry(t)

	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill() })

	inst := instance{PID: child.Process.Pid, Path: "/child"}
	file := writeInstance(t, dir, instanceFile("", inst.PID), inst)

	if err := inst.terminate(); err != nil {
		t.Fatalf("terminate: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "terminated") {
			t.Errorf("child exited with %v, expected a SIGTERM exit", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child process did not exit")
	}

	if fileExists(file) {
		t.Errorf("registry file %s was not removed", file)
	}
}

func TestTerminateDeadInstance(t *testing.T) {
	dir := setupRegistry(t)
	inst := instance{PID: 999999, Path: "/gone"}
	file := writeInstance(t, dir, "999999.json", inst)

	if err := inst.terminate(); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	if fileExists(file) {
		t.Errorf("registry file %s was not removed", file)
	}
}
