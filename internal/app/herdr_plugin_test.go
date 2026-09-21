package app

import (
	"reflect"
	"testing"
)

func TestParseCommandLine(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{
			line: "run --sandbox dev -- git status",
			want: []string{"run", "--sandbox", "dev", "--", "git", "status"},
		},
		{
			line: `upload "local path" 'remote path'`,
			want: []string{"upload", "local path", "remote path"},
		},
		{
			line: `run -- bash -lc "git status && pwd"`,
			want: []string{"run", "--", "bash", "-lc", "git status && pwd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got, err := parseCommandLine(tt.line)
			if err != nil {
				t.Fatalf("parseCommandLine() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseCommandLine() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHerdrProjectDirUsesOverride(t *testing.T) {
	t.Setenv(herdrProjectDirEnv, "/override")
	t.Setenv("HERDR_PLUGIN_CONTEXT_JSON", `{"workspace_cwd":"/workspace"}`)

	got, err := herdrProjectDir()
	if err != nil {
		t.Fatalf("herdrProjectDir() error = %v", err)
	}
	if got != "/override" {
		t.Fatalf("herdrProjectDir() = %q, want %q", got, "/override")
	}
}

func TestHerdrProjectDirPrefersWorktree(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_CONTEXT_JSON", `{
		"workspace_cwd":"/workspace",
		"focused_pane_cwd":"/pane",
		"worktree":{"checkout_path":"/worktree"}
	}`)

	got, err := herdrProjectDir()
	if err != nil {
		t.Fatalf("herdrProjectDir() error = %v", err)
	}
	if got != "/worktree" {
		t.Fatalf("herdrProjectDir() = %q, want %q", got, "/worktree")
	}
}
