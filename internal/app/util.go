package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

const maxSandboxNameLength = 19

var slugNoiseRE = regexp.MustCompile(`[^a-z0-9]+`)

func ResolveRepoContext(_ context.Context) (RepoContext, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return RepoContext{}, fmt.Errorf("get working directory: %w", err)
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return RepoContext{}, fmt.Errorf("resolve working directory: %w", err)
	}

	repoRoot, err := findRepoRoot(cwd)
	if err != nil {
		return RepoContext{}, err
	}
	repo, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return RepoContext{}, fmt.Errorf("open git repository: %w", err)
	}

	gitRef, err := currentGitRef(repo)
	if err != nil {
		return RepoContext{}, err
	}

	return RepoContext{
		RepoRoot:           repoRoot,
		RepoName:           filepath.Base(repoRoot),
		GitRef:             gitRef,
		DefaultSandboxName: slugify(filepath.Base(repoRoot) + "-" + gitRef),
		InvocationCWD:      cwd,
	}, nil
}

func findRepoRoot(path string) (string, error) {
	for {
		if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
			return path, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect git metadata: %w", err)
		}

		parent := filepath.Dir(path)
		if parent == path {
			return "", fmt.Errorf("this command must be run inside a git repository")
		}
		path = parent
	}
}

func currentGitRef(repo *git.Repository) (string, error) {
	head, err := repo.Head()
	if err == nil {
		if head.Name().IsBranch() {
			return head.Name().Short(), nil
		}
		hash := head.Hash().String()
		if len(hash) > 7 {
			return hash[:7], nil
		}
		return hash, nil
	}

	// In an empty repository or unborn branch, HEAD cannot be resolved to a commit.
	// Fall back to inspecting the symbolic HEAD reference.
	ref, refErr := repo.Reference(plumbing.HEAD, false)
	if refErr == nil {
		if ref.Type() == plumbing.SymbolicReference {
			target := ref.Target()
			if target.IsBranch() {
				return target.Short(), nil
			}
			return target.Short(), nil
		}
		if ref.Type() == plumbing.HashReference {
			hash := ref.Hash().String()
			if len(hash) > 7 {
				return hash[:7], nil
			}
			return hash, nil
		}
	}

	return "", fmt.Errorf("resolve git ref: %w", err)
}

// slugify returns a Microsandbox-compatible sandbox name. Long names retain a
// readable prefix and a stable hash suffix so distinct repository/branch pairs
// do not collapse to the same truncated name.
func slugify(value string) string {
	value = strings.ToLower(value)
	value = slugNoiseRE.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "sandbox"
	}
	if len(value) <= maxSandboxNameLength {
		return value
	}

	sum := sha256.Sum256([]byte(value))
	const hashLength = 7
	prefixLength := maxSandboxNameLength - hashLength - 1
	return strings.TrimRight(
		value[:prefixLength],
		"-",
	) + "-" + fmt.Sprintf("%x", sum[:])[:hashLength]
}

// assertLocalPathIsSafe verifies that target is within root and that resolving
// any existing component cannot leave root through a symbolic link. Call it
// immediately before modifying a path received from a sandbox or CLI argument.
func assertLocalPathIsSafe(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) ||
		filepath.IsAbs(rel) {
		return fmt.Errorf("path is outside the working directory: %s", target)
	}

	current := root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			// Descendants cannot exist if this component does not.
			break
		}
		if err != nil {
			return fmt.Errorf("inspect local download path %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write through symlink in working directory: %s", current)
		}
	}
	return nil
}

// sandboxProjectDir returns the sandbox project directory. Its final component is
// always the host repository directory name so all synchronization targets agree.
func sandboxProjectDir(rc RepoContext, remoteDir string) (string, error) {
	if remoteDir == "" {
		remoteDir = filepath.Join("/root", rc.RepoName)
	}
	remoteDir = filepath.ToSlash(filepath.Clean(remoteDir))
	if !filepath.IsAbs(remoteDir) {
		return "", fmt.Errorf("sandbox project directory must be absolute: %s", remoteDir)
	}
	if filepath.Base(remoteDir) != rc.RepoName {
		return "", fmt.Errorf(
			"sandbox project directory %q must end with the host project folder name %q",
			remoteDir,
			rc.RepoName,
		)
	}
	return remoteDir, nil
}

func resolveRemoteWorkdir(repoRoot, remoteRepoDir, invocationCWD string) (string, error) {
	if invocationCWD == repoRoot {
		return filepath.ToSlash(remoteRepoDir), nil
	}

	prefix := repoRoot + string(os.PathSeparator)
	if strings.HasPrefix(invocationCWD, prefix) {
		rel := strings.TrimPrefix(invocationCWD, prefix)
		return filepath.ToSlash(filepath.Join(remoteRepoDir, rel)), nil
	}

	return "", fmt.Errorf("current working directory is outside the git repo: %s", invocationCWD)
}

func waitDelay() time.Duration {
	return 2 * time.Second
}

func parseBool(val string, defaultVal bool) bool {
	val = strings.TrimSpace(strings.ToLower(val))
	switch val {
	case "1", "t", "true", "yes", "y", "on":
		return true
	case "0", "f", "false", "no", "n", "off":
		return false
	default:
		return defaultVal
	}
}
