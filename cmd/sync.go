// Copyright 1999-2026. WebPros International GmbH.

package cmd

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

const batchDelay = 100 * time.Millisecond

const removalChunkSize = 200

type syncItem struct {
	eventPath  string
	sourcePath string
	targetPath string
}

type syncRule struct {
	sourcePath string
	targetPath string
}

type syncQueue struct {
	mu      sync.Mutex
	pending map[syncItem]struct{}
	wake    chan struct{}
}

func newSyncQueue() *syncQueue {
	q := &syncQueue{
		pending: make(map[syncItem]struct{}),
		wake:    make(chan struct{}, 1),
	}
	go q.run()
	return q
}

func (q *syncQueue) add(item syncItem) {
	q.mu.Lock()
	q.pending[item] = struct{}{}
	q.mu.Unlock()

	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *syncQueue) take() []syncItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	items := make([]syncItem, 0, len(q.pending))
	for item := range q.pending {
		items = append(items, item)
	}
	clear(q.pending)
	return items
}

func (q *syncQueue) run() {
	for range q.wake {
		time.Sleep(batchDelay)
		if items := q.take(); len(items) > 0 {
			syncItems(items)
		}
	}
}

func syncItems(items []syncItem) {
	uploads := make(map[syncRule][]string)
	removals := make(map[syncRule][]string)
	for _, item := range items {
		rule := syncRule{sourcePath: item.sourcePath, targetPath: item.targetPath}
		if fileExists(item.eventPath) {
			uploads[rule] = append(uploads[rule], item.eventPath)
		} else {
			removals[rule] = append(removals[rule], item.eventPath)
		}
	}

	for _, rule := range sortedRules(removals) {
		removePaths(rule, collapsePaths(removals[rule]))
	}
	for _, rule := range sortedRules(uploads) {
		uploadPaths(rule, collapsePaths(uploads[rule]))
	}
}

func sortedRules(m map[syncRule][]string) []syncRule {
	rules := make([]syncRule, 0, len(m))
	for rule := range m {
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].sourcePath < rules[j].sourcePath })
	return rules
}

func collapsePaths(paths []string) []string {
	set := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		set[p] = struct{}{}
	}

	result := make([]string, 0, len(set))
	for p := range set {
		if hasAncestorIn(p, set) {
			continue
		}
		result = append(result, p)
	}
	sort.Strings(result)
	return result
}

func hasAncestorIn(p string, set map[string]struct{}) bool {
	for dir := path.Dir(p); dir != "." && dir != "/" && dir != p; dir = path.Dir(dir) {
		if _, ok := set[dir]; ok {
			return true
		}
		p = dir
	}
	return false
}

func remotePath(rule syncRule, eventPath string) string {
	return filepath.Join(rule.targetPath, strings.TrimPrefix(eventPath, rule.sourcePath))
}

func relativePath(rule syncRule, eventPath string) string {
	return strings.TrimPrefix(strings.TrimPrefix(eventPath, rule.sourcePath), "/")
}

func removePaths(rule syncRule, paths []string) {
	for chunk := range slices.Chunk(paths, removalChunkSize) {
		args := []string{remoteHost, "rm", "-rf"}
		for _, p := range chunk {
			args = append(args, fmt.Sprintf("%q", remotePath(rule, p)))
		}

		out, err := runWithSpinner(sshCommand("ssh", args...))
		if err != nil {
			log.Print(color.RedString("removal error: %s%s", err, formatCommandOutput(out)))
			continue
		}
		for _, p := range chunk {
			log.Printf("removed %s", color.YellowString("%s:%s", remoteHost, remotePath(rule, p)))
		}
	}
}

func uploadPaths(rule syncRule, paths []string) {
	baseDir := rule.sourcePath
	if baseDir == "" {
		baseDir = "."
	}
	rels := make([]string, len(paths))
	for i, p := range paths {
		rels[i] = relativePath(rule, p)
	}

	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(writeTar(pw, baseDir, rels))
	}()

	remoteCmd := fmt.Sprintf("mkdir -p %q && tar -xf - -C %q", rule.targetPath, rule.targetPath)
	cmd := sshCommand("ssh", remoteHost, remoteCmd)
	cmd.Stdin = pr
	out, err := runWithSpinner(cmd)
	_ = pr.Close()

	if err != nil {
		log.Print(color.RedString("upload error: %s%s", err, formatCommandOutput(out)))
		return
	}
	for _, p := range paths {
		log.Printf("updated %s", color.GreenString("%s:%s", remoteHost, remotePath(rule, p)))
	}
}

func runWithSpinner(cmd *exec.Cmd) ([]byte, error) {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Start()
	out, err := cmd.CombinedOutput()
	s.Stop()
	return out, err
}

func writeTar(w io.Writer, baseDir string, paths []string) error {
	tw := tar.NewWriter(w)
	for _, p := range paths {
		root := filepath.Join(baseDir, p)
		err := filepath.WalkDir(root, func(fp string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}

			rel, err := filepath.Rel(baseDir, fp)
			if err != nil {
				return err
			}
			if rel == "." {
				return nil
			}
			if isIgnored(rel) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}

			return writeTarEntry(tw, fp, filepath.ToSlash(rel))
		})
		if err != nil {
			return err
		}
	}
	return tw.Close()
}

func writeTarEntry(tw *tar.Writer, fp string, name string) error {
	info, err := os.Lstat(fp)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	link := ""
	if info.Mode()&fs.ModeSymlink != 0 {
		if link, err = os.Readlink(fp); err != nil {
			return err
		}
	}

	hdr, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return err
	}
	hdr.Name = name
	if info.IsDir() {
		hdr.Name += "/"
	}
	hdr.Uid, hdr.Gid = 0, 0
	hdr.Uname, hdr.Gname = "", ""

	if !info.Mode().IsRegular() {
		return tw.WriteHeader(hdr)
	}

	f, err := os.Open(fp)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := io.CopyN(tw, f, hdr.Size); err != nil {
		return fmt.Errorf("reading %s: %w", fp, err)
	}
	return nil
}
