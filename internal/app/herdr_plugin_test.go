package app

import (
	"reflect"
	"testing"
)

func TestHerdrOperationArgs(t *testing.T) {
	tests := []struct {
		operation string
		want      []string
		ok        bool
	}{
		{operation: "run", want: []string{"run", "--herdr", "true"}, ok: true},
		{operation: "provision", want: []string{"provision", "--herdr", "true"}, ok: true},
		{operation: "status", want: []string{"status"}, ok: true},
		{operation: "unknown", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			got, ok := herdrOperationArgs(tt.operation)
			if ok != tt.ok {
				t.Fatalf("herdrOperationArgs() ok = %v, want %v", ok, tt.ok)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("herdrOperationArgs() = %v, want %v", got, tt.want)
			}
		})
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
