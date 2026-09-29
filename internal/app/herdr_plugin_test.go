package app

import (
	"reflect"
	"strings"
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

func TestDashboardViewLayout(t *testing.T) {
	model := herdrDashboardModel{
		width: 80,
	}
	view := model.View()
	text := view.Content

	if strings.Contains(text, "[1]") || strings.Contains(text, "[2]") {
		t.Errorf("View should not contain square-bracketed numbers, got:\n%s", text)
	}

	lines := strings.Split(text, "\n")
	foundItemWithRightNumber := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		if strings.Contains(line, "Run") && strings.HasSuffix(trimmed, "1") {
			foundItemWithRightNumber = true
		}
	}
	if !foundItemWithRightNumber {
		t.Errorf("Expected 'Run' line to end with number 1, got lines:\n%s", text)
	}
}

func TestDashboardSettingsLayout(t *testing.T) {
	model := herdrDashboardModel{
		width:        80,
		settingsMode: true,
		settings: []herdrDashboardSetting{
			{label: "Mezha", path: "/tmp/mezha.toml"},
		},
	}
	view := model.View()
	text := view.Content

	if strings.Contains(text, "[1]") {
		t.Errorf("Settings view should not contain square-bracketed numbers, got:\n%s", text)
	}

	lines := strings.Split(text, "\n")
	foundSettingWithRightNumber := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		if strings.Contains(line, "Mezha") && strings.HasSuffix(trimmed, "1") {
			foundSettingWithRightNumber = true
		}
	}
	if !foundSettingWithRightNumber {
		t.Errorf("Expected settings line to end with number 1, got lines:\n%s", text)
	}
}

func TestDashboardRoutingMatchesMezhaCLI(t *testing.T) {
	expectedTopLevel := []string{
		"Run",
		"Init",
		"Sandbox…",
		"Processes…",
		"Image…",
		"Sync…",
		"Volume…",
		"Settings…",
		"Command…",
	}

	if len(herdrDashboardItems) != len(expectedTopLevel) {
		t.Fatalf(
			"herdrDashboardItems length = %d, want %d",
			len(herdrDashboardItems),
			len(expectedTopLevel),
		)
	}
	for i, expected := range expectedTopLevel {
		if herdrDashboardItems[i].title != expected {
			t.Errorf("item[%d].title = %q, want %q", i, herdrDashboardItems[i].title, expected)
		}
	}

	// Verify Sandbox submenu
	for _, item := range herdrSandboxItems {
		if len(item.args) < 2 || item.args[0] != "sandbox" {
			t.Errorf("sandbox item %q has invalid routing args: %v", item.title, item.args)
		}
	}

	// Verify Sync submenu
	for _, item := range herdrSyncItems {
		if len(item.args) < 2 || item.args[0] != "sync" {
			t.Errorf("sync item %q has invalid routing args: %v", item.title, item.args)
		}
	}

	// Verify Image submenu
	for _, item := range herdrImageItems {
		if len(item.args) < 2 || item.args[0] != "image" {
			t.Errorf("image item %q has invalid routing args: %v", item.title, item.args)
		}
	}

	// Verify Volume submenu
	for _, item := range herdrVolumeItems {
		if len(item.args) < 2 || item.args[0] != "volume" {
			t.Errorf("volume item %q has invalid routing args: %v", item.title, item.args)
		}
	}

	// Verify Processes submenu
	for _, item := range herdrProcessesItems {
		if len(item.args) < 2 || item.args[0] != "processes" {
			t.Errorf("processes item %q has invalid routing args: %v", item.title, item.args)
		}
	}
}

func TestCommandSupportsSandbox(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"run", "--herdr"}, true},
		{[]string{"init"}, false},
		{[]string{"sandbox", "list"}, false},
		{[]string{"sandbox", "create", "--herdr"}, true},
		{[]string{"sandbox", "destroy"}, true},
		{[]string{"sync", "upload"}, true},
		{[]string{"sync", "push"}, false},
		{[]string{"image", "pull"}, false},
		{[]string{"volume", "list"}, false},
		{[]string{"processes", "list"}, true},
	}

	for _, tc := range cases {
		got := commandSupportsSandbox(tc.args)
		if got != tc.want {
			t.Errorf("commandSupportsSandbox(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
