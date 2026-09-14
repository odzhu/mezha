package app

import (
	"fmt"

	"github.com/go-git/go-git/v5"
)

func gitTrackedAndUntracked(repoRoot string) ([]string, error) {
	repo, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, fmt.Errorf("open git repository: %w", err)
	}

	index, err := repo.Storer.Index()
	if err != nil {
		return nil, fmt.Errorf("read git index: %w", err)
	}

	paths := make([]string, 0, len(index.Entries))
	seen := make(map[string]struct{}, len(index.Entries))
	for _, entry := range index.Entries {
		paths = append(paths, entry.Name)
		seen[entry.Name] = struct{}{}
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("open git worktree: %w", err)
	}
	status, err := worktree.Status()
	if err != nil {
		return nil, fmt.Errorf("get git worktree status: %w", err)
	}
	for path, fileStatus := range status {
		if fileStatus.Worktree == git.Untracked {
			if _, ok := seen[path]; !ok {
				paths = append(paths, path)
			}
		}
	}
	return paths, nil
}
