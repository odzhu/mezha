package app

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/odzhu/mezha/internal/execx"
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

func unregisterSandboxGitRemote(ctx context.Context, repoRoot, remoteName string) error {
	remotes, err := execx.Output(ctx, "git", "-C", repoRoot, "remote")
	if err != nil {
		return fmt.Errorf("list Git remotes: %w", err)
	}
	for _, remote := range strings.Fields(string(remotes)) {
		if remote != remoteName {
			continue
		}
		if err := execx.Stream(ctx, repoRoot, "git", "remote", "remove", remoteName); err != nil {
			return fmt.Errorf("unregister sandbox Git remote: %w", err)
		}
		fmt.Printf("Unregistered sandbox Git remote: %s\n", remoteName)
		break
	}
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
