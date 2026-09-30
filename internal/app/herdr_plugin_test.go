package app

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
		if len(item.children) > 0 {
			if item.title != "Processes…" {
				t.Errorf("unexpected sandbox submenu item with children: %q", item.title)
			}
			continue
		}
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
	expectedProcesses := []string{"List processes", "Start…", "Stop…"}
	if len(herdrProcessesItems) != len(expectedProcesses) {
		t.Fatalf(
			"herdrProcessesItems length = %d, want %d",
			len(herdrProcessesItems),
			len(expectedProcesses),
		)
	}
	for i, expected := range expectedProcesses {
		if herdrProcessesItems[i].title != expected {
			t.Errorf(
				"processes item[%d].title = %q, want %q",
				i,
				herdrProcessesItems[i].title,
				expected,
			)
		}
	}

	// Verify Start... children
	processesMenu := buildProcessesSubmenu([]string{"processes"}, []string{"docker", "k3s"})
	startItem := processesMenu[1]
	if len(startItem.children) < 4 {
		t.Fatalf("Start… children count = %d, want >= 4", len(startItem.children))
	}
	if startItem.children[0].title != "All" ||
		!reflect.DeepEqual(startItem.children[0].args, []string{"processes", "start"}) {
		t.Errorf("Start… children[0] = %+v, want All -> processes start", startItem.children[0])
	}
	if startItem.children[1].title != "docker" ||
		!reflect.DeepEqual(startItem.children[1].args, []string{"processes", "start", "docker"}) {
		t.Errorf(
			"Start… children[1] = %+v, want docker -> processes start docker",
			startItem.children[1],
		)
	}
	if startItem.children[2].title != "k3s" ||
		!reflect.DeepEqual(startItem.children[2].args, []string{"processes", "start", "k3s"}) {
		t.Errorf("Start… children[2] = %+v, want k3s -> processes start k3s", startItem.children[2])
	}
	lastStartChild := startItem.children[len(startItem.children)-1]
	if lastStartChild.title != "Custom…" || lastStartChild.processAction != "start" {
		t.Errorf("Start… last child = %+v, want Custom… with action start", lastStartChild)
	}

	// Verify Stop... children
	stopItem := processesMenu[2]
	if len(stopItem.children) < 4 {
		t.Fatalf("Stop… children count = %d, want >= 4", len(stopItem.children))
	}
	if stopItem.children[0].title != "All" ||
		!reflect.DeepEqual(stopItem.children[0].args, []string{"processes", "stop"}) {
		t.Errorf("Stop… children[0] = %+v, want All -> processes stop", stopItem.children[0])
	}
	if stopItem.children[1].title != "docker" ||
		!reflect.DeepEqual(stopItem.children[1].args, []string{"processes", "stop", "docker"}) {
		t.Errorf(
			"Stop… children[1] = %+v, want docker -> processes stop docker",
			stopItem.children[1],
		)
	}
	if stopItem.children[2].title != "k3s" ||
		!reflect.DeepEqual(stopItem.children[2].args, []string{"processes", "stop", "k3s"}) {
		t.Errorf("Stop… children[2] = %+v, want k3s -> processes stop k3s", stopItem.children[2])
	}
	lastStopChild := stopItem.children[len(stopItem.children)-1]
	if lastStopChild.title != "Custom…" || lastStopChild.processAction != "stop" {
		t.Errorf("Stop… last child = %+v, want Custom… with action stop", lastStopChild)
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
		{[]string{"processes", "start"}, true},
		{[]string{"processes", "stop"}, true},
		{[]string{"processes", "start", "docker"}, true},
		{[]string{"processes", "stop", "docker"}, true},
	}

	for _, tc := range cases {
		got := commandSupportsSandbox(tc.args)
		if got != tc.want {
			t.Errorf("commandSupportsSandbox(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestParseProcessesListRealOutput(t *testing.T) {
	output := `docker                         stopped restarts: 0
herdr                          ready restarts: 0
k3s                            not_started restarts: 0
• Validating lock
✓ Validating lock in 6.80ms
  evaluating file '«nix-internal»/derivation-internal.nix'
`
	got := parseProcessesList(output)
	want := []string{"docker", "k3s"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseProcessesList() = %v, want %v", got, want)
	}
}

func TestParseProcessesListTableOutput(t *testing.T) {
	output := `
┌─────────┬─────────┬──────┬─────────┐
│ NAME    │ STATUS  │ PID  │ AGE     │
├─────────┼─────────┼──────┼─────────┤
│ docker  │ Running │ 45   │ 10m23s  │
│ herdr   │ Running │ 46   │ 10m23s  │
│ k3s     │ Stopped │ -    │ -       │
└─────────┴─────────┴──────┴─────────┘
`
	got := parseProcessesList(output)
	want := []string{"docker", "k3s"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseProcessesList() = %v, want %v", got, want)
	}
}

func TestDetectProcessesListWithSandbox(t *testing.T) {
	procs := detectProcessesList(t.Context(), "", "debianv4")
	if len(procs) == 0 {
		t.Skip("sandbox debianv4 is not running or binary not found")
	}
	t.Logf("detected processes from debianv4: %v", procs)
	if len(procs) != 2 || procs[0] != "docker" || procs[1] != "k3s" {
		t.Errorf("detected processes = %v, want [docker k3s]", procs)
	}
}

func TestSubmenuStackNavigation(t *testing.T) {
	model := herdrDashboardModel{
		width: 80,
	}

	// Select Processes…
	processesIndex := -1
	for i, it := range herdrDashboardItems {
		if it.title == "Processes…" {
			processesIndex = i
			break
		}
	}
	if processesIndex < 0 {
		t.Fatal("Processes… not found in herdrDashboardItems")
	}

	model.cursor = processesIndex
	matches := model.filteredDashboardItems()
	newModel, _ := model.chooseDashboardItem(matches)
	m := newModel.(herdrDashboardModel)

	if len(m.submenu) == 0 || m.submenuTitle != "Processes…" {
		t.Fatalf("expected to enter Processes… submenu, got title %q", m.submenuTitle)
	}
	if len(m.submenuStack) != 1 {
		t.Fatalf("expected submenuStack length 1, got %d", len(m.submenuStack))
	}

	// Now select Start…
	startIndex := -1
	for i, it := range m.submenu {
		if it.title == "Start…" {
			startIndex = i
			break
		}
	}
	if startIndex < 0 {
		t.Fatal("Start… not found in Processes submenu")
	}

	m.cursor = startIndex
	startMatches := m.filteredDashboardItems()
	newModel2, _ := m.chooseDashboardItem(startMatches)
	m2 := newModel2.(herdrDashboardModel)

	if len(m2.submenu) == 0 || m2.submenuTitle != "Start…" {
		t.Fatalf("expected to enter Start… submenu, got title %q", m2.submenuTitle)
	}
	if len(m2.submenuStack) != 2 {
		t.Fatalf("expected submenuStack length 2, got %d", len(m2.submenuStack))
	}

	// Press Esc to go back to Processes…
	m3 := m2.closeDashboardSubmenu()
	if m3.submenuTitle != "Processes…" {
		t.Fatalf("expected back to Processes…, got %q", m3.submenuTitle)
	}
	if len(m3.submenuStack) != 1 {
		t.Fatalf("expected submenuStack length 1, got %d", len(m3.submenuStack))
	}

	// Press Esc to go back to root
	m4 := m3.closeDashboardSubmenu()
	if m4.submenuTitle != "" || len(m4.submenu) != 0 {
		t.Fatalf(
			"expected back to root, got title %q and submenu len %d",
			m4.submenuTitle,
			len(m4.submenu),
		)
	}
	if len(m4.submenuStack) != 0 {
		t.Fatalf("expected empty submenuStack, got %d", len(m4.submenuStack))
	}
}

func TestCustomProcessInput(t *testing.T) {
	model := herdrDashboardModel{
		processMode:   true,
		processAction: "start",
		processBase:   []string{"processes"},
		sandbox:       "my-box",
	}

	// Type "redis" into process input
	for _, ch := range "redis" {
		newM, _ := model.updateProcessInput(tea.KeyPressMsg{
			Code: ch,
			Text: string(ch),
		})
		model = newM.(herdrDashboardModel)
	}

	if string(model.processInput) != "redis" {
		t.Fatalf("processInput = %q, want %q", string(model.processInput), "redis")
	}

	// Press Enter
	newM, cmd := model.updateProcessInput(tea.KeyPressMsg{
		Code: tea.KeyEnter,
	})
	model = newM.(herdrDashboardModel)
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd on enter")
	}

	expectedChosen := []string{"processes", "start", "redis", "--sandbox", "my-box"}
	if !reflect.DeepEqual(model.chosen, expectedChosen) {
		t.Fatalf("chosen = %v, want %v", model.chosen, expectedChosen)
	}
}

func TestCustomProcessInputEscapeToProcessesPlate(t *testing.T) {
	processesItems := buildProcessesSubmenu([]string{"processes"}, []string{"docker"})
	model := herdrDashboardModel{
		processMode:   true,
		processAction: "start",
		processBase:   []string{"processes"},
		submenuTitle:  "Start…",
		submenuStack: []herdrSubmenuState{
			{items: nil, title: "", cursor: 3},
			{items: processesItems, title: "Processes…", cursor: 1},
		},
	}

	// Press Esc
	newM, cmd := model.updateProcessInput(tea.KeyPressMsg{
		Code: tea.KeyEsc,
	})
	m := newM.(herdrDashboardModel)
	if cmd != nil {
		t.Fatal("expected nil cmd on esc")
	}
	if m.processMode {
		t.Fatal("expected processMode to be false")
	}
	if m.submenuTitle != "Processes…" {
		t.Fatalf("expected submenuTitle to be Processes…, got %q", m.submenuTitle)
	}
	if len(m.submenu) != len(processesItems) {
		t.Fatalf("expected submenu to be processes items, len = %d", len(m.submenu))
	}
}

func TestRestoreSubmenuForProcessesCommand(t *testing.T) {
	model := herdrDashboardModel{
		dashboardItems: buildDashboardItems([]string{"docker", "k3s"}),
	}

	restoreSubmenuForCommand(&model, []string{"processes", "start", "docker"})

	if model.submenuTitle != "Processes…" {
		t.Fatalf("expected submenuTitle = Processes…, got %q", model.submenuTitle)
	}
	if len(model.submenu) == 0 {
		t.Fatal("expected submenu to be populated")
	}
	if len(model.submenuStack) != 1 {
		t.Fatalf("expected submenuStack len = 1, got %d", len(model.submenuStack))
	}

	// Pressing esc from Processes… plate returns to main plate
	m := model.closeDashboardSubmenu()
	if m.submenuTitle != "" || len(m.submenu) != 0 {
		t.Fatalf(
			"expected main plate, got title %q and submenu len %d",
			m.submenuTitle,
			len(m.submenu),
		)
	}
}
