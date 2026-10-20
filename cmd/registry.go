// Copyright 1999-2026. WebPros International GmbH.

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type instance struct {
	PID        int    `json:"pid"`
	Path       string `json:"path"`
	RemoteHost string `json:"remote_host"`
}

const terminateTimeout = 3 * time.Second

func registryDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to locate the home directory: %w", err)
	}
	return filepath.Join(home, ".psync"), nil
}

func instanceFile(dir string, pid int) string {
	return filepath.Join(dir, strconv.Itoa(pid)+".json")
}

func canonicalPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

func (inst instance) isAlive() bool {
	return syscall.Kill(inst.PID, 0) == nil
}

func registerInstance(watchPath string, host string) (func(), error) {
	dir, err := registryDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("unable to create %s: %w", dir, err)
	}

	data, err := json.Marshal(instance{PID: os.Getpid(), Path: canonicalPath(watchPath), RemoteHost: host})
	if err != nil {
		return nil, err
	}
	file := instanceFile(dir, os.Getpid())
	if err := os.WriteFile(file, data, 0o600); err != nil {
		return nil, fmt.Errorf("unable to register the watcher: %w", err)
	}

	return func() { _ = os.Remove(file) }, nil
}

func loadInstances() ([]instance, error) {
	dir, err := registryDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("unable to read %s: %w", dir, err)
	}

	var instances []instance
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		file := filepath.Join(dir, entry.Name())

		inst, err := readInstance(file)
		if err != nil || !inst.isAlive() {
			_ = os.Remove(file)
			continue
		}
		instances = append(instances, inst)
	}

	sort.Slice(instances, func(i, j int) bool {
		if instances[i].Path != instances[j].Path {
			return instances[i].Path < instances[j].Path
		}
		return instances[i].PID < instances[j].PID
	})
	return instances, nil
}

func readInstance(file string) (instance, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return instance{}, err
	}

	var inst instance
	if err := json.Unmarshal(data, &inst); err != nil {
		return instance{}, err
	}
	if inst.PID <= 0 || inst.Path == "" {
		return instance{}, errors.New("malformed registry entry")
	}
	if pid, err := strconv.Atoi(strings.TrimSuffix(filepath.Base(file), ".json")); err != nil || pid != inst.PID {
		return instance{}, errors.New("registry entry does not match its file name")
	}
	return inst, nil
}

func (inst instance) terminate() error {
	if err := syscall.Kill(inst.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("unable to stop process %d: %w", inst.PID, err)
	}

	deadline := time.Now().Add(terminateTimeout)
	for inst.isAlive() {
		if time.Now().After(deadline) {
			if err := syscall.Kill(inst.PID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				return fmt.Errorf("unable to kill process %d: %w", inst.PID, err)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if dir, err := registryDir(); err == nil {
		_ = os.Remove(instanceFile(dir, inst.PID))
	}
	return nil
}
