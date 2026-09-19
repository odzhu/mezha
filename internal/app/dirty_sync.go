package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const syncedUntrackedPathsFile = ".mezha/dirty-sync-untracked.json"

type dirtyPaths struct {
	copy   []string
	delete []string
}

// trackedDirtyPaths returns paths whose working-tree state differs from HEAD.
func trackedDirtyPaths(ctx context.Context, repoRoot string) (dirtyPaths, error) {
	output, err := gitOutput(ctx, repoRoot, "diff", "--name-status", "-z", "HEAD")
	if err != nil {
		return dirtyPaths{}, fmt.Errorf("find dirty tracked files: %w", err)
	}
	dirty, err := parseDirtyPaths(output)
	if err != nil {
		return dirtyPaths{}, fmt.Errorf("parse dirty tracked files: %w", err)
	}
	untracked, err := localUntrackedPaths(ctx, repoRoot)
	if err != nil {
		return dirtyPaths{}, err
	}
	dirty.copy = append(dirty.copy, untracked...)
	return dirty, nil
}

func loadSyncedUntrackedPaths(repoRoot string) ([]string, error) {
	contents, err := os.ReadFile(filepath.Join(repoRoot, syncedUntrackedPathsFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read synced untracked paths: %w", err)
	}
	var paths []string
	if err := json.Unmarshal(contents, &paths); err != nil {
		return nil, fmt.Errorf("parse synced untracked paths: %w", err)
	}
	for _, relativePath := range paths {
		if !validRepoRelativePath(relativePath) {
			return nil, fmt.Errorf("invalid synced untracked path %q", relativePath)
		}
	}
	return paths, nil
}

func localUntrackedPaths(ctx context.Context, repoRoot string) ([]string, error) {
	output, err := gitOutput(ctx, repoRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("find untracked files: %w", err)
	}
	paths := make([]string, 0)
	for _, item := range bytesSplit(output, 0) {
		relativePath := string(item)
		if relativePath == "" {
			continue
		}
		if !validRepoRelativePath(relativePath) {
			return nil, fmt.Errorf("invalid untracked path %q", relativePath)
		}
		paths = append(paths, relativePath)
	}
	return paths, nil
}

func saveSyncedUntrackedPaths(repoRoot string, untracked []string) error {
	paths := append([]string(nil), untracked...)
	sort.Strings(paths)
	contents, err := json.Marshal(paths)
	if err != nil {
		return fmt.Errorf("encode synced untracked paths: %w", err)
	}
	statePath := filepath.Join(repoRoot, syncedUntrackedPathsFile)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return fmt.Errorf("create synced untracked path directory: %w", err)
	}
	if err := os.WriteFile(statePath, contents, 0o600); err != nil {
		return fmt.Errorf("save synced untracked paths: %w", err)
	}
	return nil
}

func parseDirtyPaths(output []byte) (dirtyPaths, error) {
	items := bytesSplit(output, 0)
	var dirty dirtyPaths
	for len(items) > 0 {
		status := string(items[0])
		items = items[1:]
		if status == "" {
			continue
		}
		if len(items) == 0 {
			return dirtyPaths{}, fmt.Errorf("missing path for status %q", status)
		}
		oldPath := string(items[0])
		items = items[1:]
		if !validRepoRelativePath(oldPath) {
			return dirtyPaths{}, fmt.Errorf("invalid path %q", oldPath)
		}

		switch status[0] {
		case 'D':
			dirty.delete = append(dirty.delete, oldPath)
		case 'R', 'C':
			if len(items) == 0 {
				return dirtyPaths{}, fmt.Errorf("missing destination path for status %q", status)
			}
			newPath := string(items[0])
			items = items[1:]
			if !validRepoRelativePath(newPath) {
				return dirtyPaths{}, fmt.Errorf("invalid path %q", newPath)
			}
			if status[0] == 'R' {
				dirty.delete = append(dirty.delete, oldPath)
			}
			dirty.copy = append(dirty.copy, newPath)
		default:
			dirty.copy = append(dirty.copy, oldPath)
		}
	}
	return dirty, nil
}

func gitOutput(ctx context.Context, repoRoot string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf(
				"git %s: %s",
				strings.Join(args, " "),
				strings.TrimSpace(string(exitErr.Stderr)),
			)
		}
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return output, nil
}

func bytesSplit(b []byte, sep byte) [][]byte {
	if len(b) == 0 {
		return nil
	}
	var result [][]byte
	for {
		i := 0
		for i < len(b) && b[i] != sep {
			i++
		}
		if i == len(b) {
			return append(result, b)
		}
		result = append(result, b[:i])
		b = b[i+1:]
	}
}

func validRepoRelativePath(relativePath string) bool {
	return relativePath != "" && !strings.ContainsRune(relativePath, '\x00') &&
		!strings.HasPrefix(relativePath, "/") && path.Clean(relativePath) == relativePath &&
		relativePath != "." && !strings.HasPrefix(relativePath, "../")
}
