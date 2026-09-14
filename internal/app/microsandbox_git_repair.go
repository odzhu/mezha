package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

func repairMicrosandboxGitRemote(_ context.Context, rc RepoContext, params GitParams, replace bool) error {
	host, err := ensureSandboxSSHConfig("", "", params.SandboxName)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("ssh://root@%s//%s/.git", host, strings.TrimPrefix(filepath.ToSlash(params.RemoteRepoDir), "/"))
	return setSandboxGitRemote(context.Background(), rc.RepoRoot, params.SandboxName, url, replace)
}
