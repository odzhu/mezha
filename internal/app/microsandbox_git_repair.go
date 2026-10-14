package app

import (
	"context"
)

func repairMicrosandboxGitRemote(
	_ context.Context,
	rc RepoContext,
	params GitParams,
	replace bool,
) error {
	host, err := ensureSandboxSSHConfig("", "", params.SandboxName)
	if err != nil {
		return err
	}
	url := sandboxGitURL(host, rc, params.RemoteRepoDir)
	return setSandboxGitRemote(context.Background(), rc.RepoRoot, params.SandboxName, url, replace)
}
