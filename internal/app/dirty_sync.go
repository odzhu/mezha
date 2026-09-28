package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
)

const syncedUntrackedPathsFile = ".mezha/dirty-sync-untracked.json"

func ignoredSyncPath(relativePath string) bool {
	return relativePath == "devenv.lock"
}

type dirtyPaths struct {
	copy   []string
	delete []string
}

func localWorktreeStatus(repoRoot string) (git.Status, error) {
	repo, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, fmt.Errorf("open git repository: %w", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("open git worktree: %w", err)
	}
	status, err := worktree.Status()
	if err != nil {
		return nil, fmt.Errorf("get git worktree status: %w", err)
	}
	return status, nil
}

// trackedDirtyPaths returns paths whose working-tree state differs from HEAD.
func trackedDirtyPaths(_ context.Context, repoRoot string) (dirtyPaths, error) {
	status, err := localWorktreeStatus(repoRoot)
	if err != nil {
		return dirtyPaths{}, fmt.Errorf("find dirty tracked files: %w", err)
	}
	var dirty dirtyPaths
	for p, fileStatus := range status {
		if !validRepoRelativePath(p) || ignoredSyncPath(p) {
			continue
		}
		if fileStatus.Worktree == git.Deleted ||
			(fileStatus.Staging == git.Deleted && fileStatus.Worktree == git.Unmodified) {
			dirty.delete = append(dirty.delete, p)
		} else if fileStatus.Worktree == git.Untracked || fileStatus.Worktree != git.Unmodified ||
			fileStatus.Staging != git.Unmodified {
			dirty.copy = append(dirty.copy, p)
		}
	}
	sort.Strings(dirty.copy)
	sort.Strings(dirty.delete)
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
	filtered := paths[:0]
	for _, relativePath := range paths {
		if !validRepoRelativePath(relativePath) {
			return nil, fmt.Errorf("invalid synced untracked path %q", relativePath)
		}
		if !ignoredSyncPath(relativePath) {
			filtered = append(filtered, relativePath)
		}
	}
	return filtered, nil
}

func localUntrackedPaths(_ context.Context, repoRoot string) ([]string, error) {
	status, err := localWorktreeStatus(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("find untracked files: %w", err)
	}
	var paths []string
	for p, fileStatus := range status {
		if fileStatus.Worktree == git.Untracked {
			if !validRepoRelativePath(p) {
				return nil, fmt.Errorf("invalid untracked path %q", p)
			}
			if !ignoredSyncPath(p) {
				paths = append(paths, p)
			}
		}
	}
	sort.Strings(paths)
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
			if !ignoredSyncPath(oldPath) {
				dirty.delete = append(dirty.delete, oldPath)
			}
		case 'R', 'C':
			if len(items) == 0 {
				return dirtyPaths{}, fmt.Errorf("missing destination path for status %q", status)
			}
			newPath := string(items[0])
			items = items[1:]
			if !validRepoRelativePath(newPath) {
				return dirtyPaths{}, fmt.Errorf("invalid path %q", newPath)
			}
			if status[0] == 'R' && !ignoredSyncPath(oldPath) {
				dirty.delete = append(dirty.delete, oldPath)
			}
			if !ignoredSyncPath(newPath) {
				dirty.copy = append(dirty.copy, newPath)
			}
		default:
			if !ignoredSyncPath(oldPath) {
				dirty.copy = append(dirty.copy, oldPath)
			}
		}
	}
	return dirty, nil
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
