package app

import "time"

type RepoContext struct {
	RepoRoot           string
	RepoName           string
	GitRef             string
	DefaultSandboxName string
	InvocationCWD      string
}

type RunParams struct {
	SandboxName          string
	RemoteRepoDir        string
	Recreate             bool
	Kubernetes           bool
	ReplaceSandboxRemote bool
	Editor               string
	RemoteCommand        []string
	TTY                  *bool
	PolicyAdvisor        *bool
	NoLoginShell         bool
	Herdr                bool
	VolumesFlush         bool
}

type GitParams struct {
	SandboxName   string
	RemoteRepoDir string
}

type TransferParams struct {
	SandboxName   string
	RemoteRepoDir string
	Recreate      bool
}

// UploadParams and DownloadParams are retained for callers of the previous
// repository-wide transfer API. New CLI commands use TransferParams.
type UploadParams struct {
	SandboxName   string
	RemoteRepoDir string
	Recreate      bool
}

type DownloadParams struct {
	SandboxName   string
	RemoteRepoDir string
}

type LogsParams struct {
	SandboxName string
	Tail        uint
	Follow      bool
	Since       time.Duration
	Sources     []string
	MinLevel    string
}

type DestroyParams struct {
	SandboxName  string
	Force        bool
	VolumesFlush bool
}
