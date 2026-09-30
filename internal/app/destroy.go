package app

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
)

// Destroy removes the project's Microsandbox and its Git remote.
func Destroy(ctx context.Context, rc RepoContext, params DestroyParams) error {
	proceed, err := destroyMicrosandbox(ctx, params)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}
	if _, err := unregisterHerdrMachine(ctx, params.SandboxName); err != nil {
		return err
	}
	if params.VolumesFlush {
		if err := flushMicrosandboxVolumes(ctx, params.SandboxName); err != nil {
			return err
		}
	}
	if err := unregisterSandboxGitRemote(ctx, rc.RepoRoot, params.SandboxName); err != nil {
		return err
	}
	return nil
}

// UnregisterSandboxGitRemote removes the sandbox Git remote from the local repository.
func UnregisterSandboxGitRemote(ctx context.Context, rc RepoContext, params GitParams) error {
	return unregisterSandboxGitRemote(ctx, rc.RepoRoot, params.SandboxName)
}

func unregisterSandboxGitRemote(_ context.Context, repoRoot, remoteName string) error {
	repo, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return fmt.Errorf("open git repository: %w", err)
	}
	if _, err := repo.Remote(remoteName); err != nil {
		return nil
	}
	if err := repo.DeleteRemote(remoteName); err != nil {
		return fmt.Errorf("unregister sandbox Git remote: %w", err)
	}
	fmt.Printf("Unregistered sandbox Git remote: %s\n", remoteName)
	return nil
}

func confirmDestroy(sandboxName string) (bool, error) {
	fmt.Printf("Delete Microsandbox %q? [y/N]: ", sandboxName)
	return confirmDeletion()
}

func confirmVolumeDestroy(volumeName string) (bool, error) {
	fmt.Printf("Delete Microsandbox volume %q? [y/N]: ", volumeName)
	return confirmDeletion()
}

func confirmDeletion() (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
