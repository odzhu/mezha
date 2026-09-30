package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
	cli "github.com/urfave/cli/v3"
)

type herdrDashboardItem struct {
	args          []string
	title         string
	description   string
	custom        bool
	settings      bool
	processAction string
	processBase   []string
	children      []herdrDashboardItem
}

type herdrSubmenuState struct {
	items  []herdrDashboardItem
	title  string
	cursor int
}

type herdrDashboardSetting struct {
	label string
	path  string
}

var (
	dashboardTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#11111B")).
				Background(lipgloss.Color("#A78BFA")).
				Padding(0, 1)
	dashboardPromptStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)
	dashboardNameStyle    = lipgloss.NewStyle()
	dashboardNameSelStyle = lipgloss.NewStyle().Bold(true)
	dashboardDescStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	dashboardBarStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)
	dashboardFooterStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	dashboardErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	dashboardDetailStyle  = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("5")).
				Padding(0, 1)
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripAnsi(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

func isValidProcessName(name string) bool {
	if name == "" || name == "herdr" {
		return false
	}
	switch strings.ToLower(name) {
	case "name", "process", "status", "pid", "age", "exit", "code", "restarts":
		return false
	}
	for _, ch := range name {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') &&
			ch != '-' &&
			ch != '_' &&
			ch != '.' {
			return false
		}
	}
	return true
}

func parseProcessesList(output string) []string {
	var names []string
	seen := make(map[string]bool)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = stripAnsi(line)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "•") ||
			strings.HasPrefix(line, "✓") ||
			strings.HasPrefix(line, "×") ||
			strings.HasPrefix(line, "evaluating") ||
			strings.HasPrefix(line, "Running") ||
			strings.HasPrefix(line, "┌") ||
			strings.HasPrefix(line, "├") ||
			strings.HasPrefix(line, "└") ||
			strings.HasPrefix(line, "+-") ||
			strings.HasPrefix(line, "--") {
			continue
		}

		if strings.Contains(line, "│") || strings.Contains(line, "|") {
			sep := "│"
			if !strings.Contains(line, "│") {
				sep = "|"
			}
			parts := strings.Split(line, sep)
			if len(parts) >= 2 {
				name := strings.TrimSpace(parts[1])
				if isValidProcessName(name) && !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
			}
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 2 {
			name := fields[0]
			if isValidProcessName(name) && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

func detectProcessesList(ctx context.Context, projectDir string, sandbox string) []string {
	binary, err := os.Executable()
	if err != nil || strings.HasSuffix(binary, ".test") {
		if path, err := exec.LookPath("mezha"); err == nil {
			binary = path
		} else if _, statErr := os.Stat("./bin/mezha"); statErr == nil {
			binary = "./bin/mezha"
		} else {
			return nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	args := []string{"processes"}
	if sandbox != "" {
		args = append(args, "--sandbox", sandbox)
	}
	args = append(args, "list")

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = projectDir
	out, _ := cmd.CombinedOutput()
	return parseProcessesList(string(out))
}

func buildProcessesSubmenu(baseArgs []string, processes []string) []herdrDashboardItem {
	startChildren := make([]herdrDashboardItem, 0, len(processes)+2)
	startChildren = append(startChildren, herdrDashboardItem{
		args:        append(append([]string(nil), baseArgs...), "start"),
		title:       "All",
		description: "Start all background devenv processes",
	})
	for _, proc := range processes {
		startChildren = append(startChildren, herdrDashboardItem{
			args:        append(append([]string(nil), baseArgs...), "start", proc),
			title:       proc,
			description: fmt.Sprintf("Start %s process in the sandbox", proc),
		})
	}
	startChildren = append(startChildren, herdrDashboardItem{
		title:         "Custom…",
		description:   "Start an individual process by name",
		processAction: "start",
		processBase:   baseArgs,
	})

	stopChildren := make([]herdrDashboardItem, 0, len(processes)+2)
	stopChildren = append(stopChildren, herdrDashboardItem{
		args:        append(append([]string(nil), baseArgs...), "stop"),
		title:       "All",
		description: "Stop all background devenv processes",
	})
	for _, proc := range processes {
		stopChildren = append(stopChildren, herdrDashboardItem{
			args:        append(append([]string(nil), baseArgs...), "stop", proc),
			title:       proc,
			description: fmt.Sprintf("Stop %s process in the sandbox", proc),
		})
	}
	stopChildren = append(stopChildren, herdrDashboardItem{
		title:         "Custom…",
		description:   "Stop an individual process by name",
		processAction: "stop",
		processBase:   baseArgs,
	})

	return []herdrDashboardItem{
		{
			args:        append(append([]string(nil), baseArgs...), "list"),
			title:       "List processes",
			description: "List background devenv processes in the sandbox",
		},
		{
			title:       "Start…",
			description: "Start all or individual background processes",
			children:    startChildren,
		},
		{
			title:       "Stop…",
			description: "Stop all or individual background processes",
			children:    stopChildren,
		},
	}
}

func buildSandboxItems(processes []string) []herdrDashboardItem {
	return []herdrDashboardItem{
		{
			args:        []string{"sandbox", "list"},
			title:       "List sandboxes",
			description: "Show all local Mezha sandboxes",
		},
		{
			args:        []string{"sandbox", "create", "--herdr"},
			title:       "Create sandbox",
			description: "Create and initialize the configured sandbox",
		},
		{
			args:        []string{"sandbox", "recreate", "--herdr"},
			title:       "Recreate",
			description: "Recreate the sandbox with clean persistent state",
		},
		{args: []string{"sandbox", "start"}, title: "Start", description: "Start the sandbox"},
		{args: []string{"sandbox", "stop"}, title: "Stop", description: "Stop the sandbox"},
		{
			args:        []string{"sandbox", "destroy"},
			title:       "Destroy",
			description: "Confirm and permanently remove the sandbox",
		},
		{
			args:        []string{"sandbox", "status"},
			title:       "Show status",
			description: "Inspect repository synchronization status",
		},
		{
			args:        []string{"sandbox", "logs"},
			title:       "Show logs",
			description: "Show Microsandbox service and runtime logs",
		},
		{
			title:       "Processes…",
			description: "Manage background devenv processes in the sandbox",
			children:    buildProcessesSubmenu([]string{"sandbox", "processes"}, processes),
		},
	}
}

var herdrProcessesItems = buildProcessesSubmenu([]string{"processes"}, nil)
var herdrSandboxItems = buildSandboxItems(nil)

var herdrImageItems = []herdrDashboardItem{
	{
		args:        []string{"image", "pull"},
		title:       "Pull latest image",
		description: "Refresh the cached native Debian image",
	},
}

var herdrSyncItems = []herdrDashboardItem{
	{
		args:        []string{"sync", "status"},
		title:       "Show status",
		description: "Inspect repository synchronization status",
	},
	{
		args:        []string{"sync", "upload"},
		title:       "Upload changes",
		description: "Send local dirty changes to the sandbox",
	},
	{
		args:        []string{"sync", "download"},
		title:       "Download changes",
		description: "Bring sandbox dirty changes into the workspace",
	},
	{
		args:        []string{"sync", "pull"},
		title:       "Pull commits",
		description: "Pull committed changes from the sandbox",
	},
	{
		args:        []string{"sync", "push"},
		title:       "Push commits",
		description: "Push committed changes to the sandbox",
	},
	{
		args:        []string{"sync", "remote", "repair"},
		title:       "Repair remote",
		description: "Refresh the sandbox remote SSH configuration",
	},
	{
		args:        []string{"sync", "remote", "deregister"},
		title:       "Deregister remote",
		description: "Remove the sandbox Git remote from the local repository",
	},
}

var herdrVolumeItems = []herdrDashboardItem{
	{
		args:        []string{"volume", "list"},
		title:       "List volumes",
		description: "Show persistent Microsandbox volumes",
	},
}

func buildDashboardItems(processes []string) []herdrDashboardItem {
	return []herdrDashboardItem{
		{
			args:        []string{"run", "--herdr"},
			title:       "Run",
			description: "Open an interactive shell in a new tab",
		},
		{
			args:        []string{"init"},
			title:       "Init",
			description: "Create the project Mezha configuration",
		},
		{
			title:       "Sandbox…",
			description: "Manage sandboxes and instances",
			children:    buildSandboxItems(processes),
		},
		{
			title:       "Processes…",
			description: "Manage devenv background processes",
			children:    buildProcessesSubmenu([]string{"processes"}, processes),
		},
		{
			title:       "Image…",
			description: "Manage Microsandbox images",
			children:    herdrImageItems,
		},
		{
			title:       "Sync…",
			description: "Synchronize repository changes with a sandbox",
			children:    herdrSyncItems,
		},
		{
			title:       "Volume…",
			description: "Manage persistent Microsandbox volumes",
			children:    herdrVolumeItems,
		},
		{
			title:       "Settings…",
			description: "Edit the Mezha or managed devenv configuration",
			settings:    true,
		},
		{
			title:       "Command…",
			description: "Run any Mezha command with arguments",
			custom:      true,
		},
	}
}

var herdrDashboardItems = buildDashboardItems(nil)

type herdrDashboardModel struct {
	cursor             int
	chosen             []string
	commandMode        bool
	filterMode         bool
	forceInitMode      bool
	settingsMode       bool
	sandboxMode        bool
	sandboxCustomMode  bool
	processMode        bool
	processAction      string
	processBase        []string
	processInput       []rune
	processInputCursor int
	input              []rune
	inputCursor        int
	filter             []rune
	settings           []herdrDashboardSetting
	settingCursor      int
	chosenSetting      string
	settingsNeedInit   bool
	submenu            []herdrDashboardItem
	submenuTitle       string
	submenuStack       []herdrSubmenuState
	sandbox            string
	sandboxes          []string
	syncedSandboxes    map[string]bool
	repoInitialized    bool
	sandboxCursor      int
	sandboxInput       []rune
	sandboxInputCursor int
	projectDir         string
	processes          []string
	dashboardItems     []herdrDashboardItem
	width              int
	height             int
	err                string
}

func (m herdrDashboardModel) Init() tea.Cmd {
	return nil
}

func (m herdrDashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if m.processMode {
		return m.updateProcessInput(key)
	}
	if m.commandMode {
		return m.updateCommand(key)
	}
	if m.forceInitMode {
		return m.updateForceInit(key)
	}
	if m.settingsMode {
		return m.updateSettings(key)
	}
	if m.sandboxCustomMode {
		return m.updateSandboxInput(key)
	}
	if m.sandboxMode {
		return m.updateSandbox(key)
	}
	if m.filterMode {
		return m.updateFilter(key)
	}
	matches := m.filteredDashboardItems()
	if index, ok := dashboardDigitIndex(key.String()); ok && index < len(matches) {
		m.cursor = index
		return m.chooseDashboardItem(matches)
	}
	switch key.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
		if len(m.submenu) > 0 || len(m.submenuStack) > 0 {
			return m.closeDashboardSubmenu(), nil
		}
		return m, tea.Quit
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down":
		if m.cursor < len(matches)-1 {
			m.cursor++
		}
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		if len(matches) > 0 {
			m.cursor = len(matches) - 1
		}
	case "/":
		m.filterMode = true
	case "space":
		m.sandboxMode = true
		m.sandboxCursor = len(m.sandboxes)
		for index, sandbox := range m.sandboxes {
			if sandbox == m.sandbox {
				m.sandboxCursor = index
				break
			}
		}
		m.err = ""
	case "enter":
		return m.chooseDashboardItem(matches)
	default:
		runes := []rune(key.Key().Text)
		if len(runes) > 0 {
			m.filterMode = true
			m.filter = append(m.filter, runes...)
			m.cursor = 0
		}
	}
	return m, nil
}

func (m herdrDashboardModel) chooseDashboardItem(matches []int) (tea.Model, tea.Cmd) {
	if m.cursor >= len(matches) {
		return m, nil
	}
	items := m.activeDashboardItems()
	item := items[matches[m.cursor]]
	if len(item.children) > 0 {
		m.submenuStack = append(m.submenuStack, herdrSubmenuState{
			items:  m.submenu,
			title:  m.submenuTitle,
			cursor: m.cursor,
		})
		m.submenu = item.children
		m.submenuTitle = item.title
		m.cursor = 0
		m.filter = nil
		m.filterMode = false
		return m, nil
	}
	if item.processAction != "" {
		m.processMode = true
		m.processAction = item.processAction
		m.processBase = item.processBase
		m.processInput = nil
		m.processInputCursor = 0
		m.filterMode = false
		m.err = ""
		return m, nil
	}
	if item.custom {
		m.commandMode = true
		m.filterMode = false
		m.err = ""
		return m, nil
	}
	if item.settings {
		m.settingsMode = true
		m.settingCursor = 0
		m.filterMode = false
		return m, nil
	}
	if len(item.args) > 0 && item.args[0] == "init" && m.repoInitialized {
		m.forceInitMode = true
		m.filterMode = false
		return m, nil
	}
	m.chosen = append([]string(nil), item.args...)
	if m.sandbox != "" && commandSupportsSandbox(item.args) {
		m.chosen = append(m.chosen, "--sandbox", m.sandbox)
	}
	return m, tea.Quit
}

func commandSupportsSandbox(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "init", "image", "volume":
		return false
	case "sandbox":
		if len(args) > 1 && args[1] == "list" {
			return false
		}
		return true
	case "sync":
		if len(args) > 1 && (args[1] == "push" || args[1] == "pull" || args[1] == "remote") {
			return false
		}
		return true
	case "run", "processes":
		return true
	}
	return false
}

func (m herdrDashboardModel) updateForceInit(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "Y":
		m.chosen = []string{"init", "--force"}
		return m, tea.Quit
	case "n", "N", "enter", "esc":
		m.forceInitMode = false
	}
	return m, nil
}

func (m herdrDashboardModel) updateSettings(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if index, ok := dashboardDigitIndex(key.String()); ok && index < len(m.settings) {
		m.chosenSetting = m.settings[index].path
		return m, tea.Quit
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.settingsMode = false
		return m, nil
	case "up", "k":
		if m.settingCursor > 0 {
			m.settingCursor--
		}
	case "down", "j":
		if m.settingCursor < len(m.settings)-1 {
			m.settingCursor++
		}
	case "home", "g":
		m.settingCursor = 0
	case "end", "G":
		if len(m.settings) > 0 {
			m.settingCursor = len(m.settings) - 1
		}
	case "enter":
		if m.settingCursor < len(m.settings) {
			m.chosenSetting = m.settings[m.settingCursor].path
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m herdrDashboardModel) closeDashboardSubmenu() herdrDashboardModel {
	if len(m.submenuStack) > 0 {
		last := m.submenuStack[len(m.submenuStack)-1]
		m.submenuStack = m.submenuStack[:len(m.submenuStack)-1]
		m.submenu = last.items
		m.submenuTitle = last.title
		m.cursor = last.cursor
		m.filter = nil
		m.filterMode = false
		return m
	}
	m.submenu = nil
	m.submenuTitle = ""
	m.cursor = 0
	m.filter = nil
	m.filterMode = false
	return m
}

func (m herdrDashboardModel) updateFilter(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	matches := m.filteredDashboardItems()
	if index, ok := dashboardDigitIndex(key.String()); ok && index < len(matches) {
		m.cursor = index
		return m.chooseDashboardItem(matches)
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.filterMode = false
		m.filter = nil
		m.cursor = 0
		return m, nil
	case "up", "ctrl+p":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "ctrl+n":
		if m.cursor < len(matches)-1 {
			m.cursor++
		}
	case "home":
		m.cursor = 0
	case "end":
		if len(matches) > 0 {
			m.cursor = len(matches) - 1
		}
	case "backspace", "ctrl+h":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.cursor = 0
		}
	case "enter":
		return m.chooseDashboardItem(matches)
	default:
		runes := []rune(key.Key().Text)
		if len(runes) > 0 {
			m.filter = append(m.filter, runes...)
			m.cursor = 0
		}
	}
	return m, nil
}

func (m herdrDashboardModel) activeDashboardItems() []herdrDashboardItem {
	if len(m.submenu) > 0 {
		return m.submenu
	}
	if len(m.dashboardItems) > 0 {
		return m.dashboardItems
	}
	return herdrDashboardItems
}

func (m herdrDashboardModel) filteredDashboardItems() []int {
	query := strings.TrimSpace(string(m.filter))
	items := m.activeDashboardItems()
	matches := make([]int, 0, len(items))
	for index, item := range items {
		candidate := item.title + " " + item.description + " " + strings.Join(item.args, " ")
		if fuzzyMatch(query, candidate) {
			matches = append(matches, index)
		}
	}
	return matches
}

func dashboardDigitIndex(key string) (int, bool) {
	if len(key) != 1 || key[0] < '0' || key[0] > '9' {
		return 0, false
	}
	if key == "0" {
		return 9, true
	}
	return int(key[0] - '1'), true
}

func dashboardDigitLabel(index int) string {
	if index == 9 {
		return "0"
	}
	return fmt.Sprintf("%d", index+1)
}

func truncateDashboardText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit < 2 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func fuzzyMatch(query, candidate string) bool {
	queryRunes := []rune(strings.ToLower(query))
	if len(queryRunes) == 0 {
		return true
	}
	matched := 0
	for _, char := range strings.ToLower(candidate) {
		if char == queryRunes[matched] {
			matched++
			if matched == len(queryRunes) {
				return true
			}
		}
	}
	return false
}

func (m herdrDashboardModel) updateSandbox(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	itemCount := len(m.sandboxes) + 1
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.sandboxMode = false
		return m, nil
	case "up", "k":
		if m.sandboxCursor > 0 {
			m.sandboxCursor--
		}
	case "down", "j":
		if m.sandboxCursor < itemCount-1 {
			m.sandboxCursor++
		}
	case "home", "g":
		m.sandboxCursor = 0
	case "end", "G":
		m.sandboxCursor = itemCount - 1
	case "enter":
		if m.sandboxCursor < len(m.sandboxes) {
			m.sandbox = m.sandboxes[m.sandboxCursor]
			m.sandboxMode = false
			m.processes = detectProcessesList(context.Background(), m.projectDir, m.sandbox)
			m.dashboardItems = buildDashboardItems(m.processes)
			return m, nil
		}
		m.sandboxMode = false
		m.sandboxCustomMode = true
		m.sandboxInput = []rune(m.sandbox)
		m.sandboxInputCursor = len(m.sandboxInput)
	}
	return m, nil
}

func (m herdrDashboardModel) updateSandboxInput(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.sandboxCustomMode = false
		m.sandboxMode = true
		return m, nil
	case "left":
		if m.sandboxInputCursor > 0 {
			m.sandboxInputCursor--
		}
	case "right":
		if m.sandboxInputCursor < len(m.sandboxInput) {
			m.sandboxInputCursor++
		}
	case "home", "ctrl+a":
		m.sandboxInputCursor = 0
	case "end", "ctrl+e":
		m.sandboxInputCursor = len(m.sandboxInput)
	case "backspace", "ctrl+h":
		if m.sandboxInputCursor > 0 {
			m.sandboxInput = append(
				m.sandboxInput[:m.sandboxInputCursor-1],
				m.sandboxInput[m.sandboxInputCursor:]...,
			)
			m.sandboxInputCursor--
		}
	case "delete", "ctrl+d":
		if m.sandboxInputCursor < len(m.sandboxInput) {
			m.sandboxInput = append(
				m.sandboxInput[:m.sandboxInputCursor],
				m.sandboxInput[m.sandboxInputCursor+1:]...,
			)
		}
	case "enter":
		m.sandbox = strings.TrimSpace(string(m.sandboxInput))
		m.sandboxCustomMode = false
		m.err = ""
		m.processes = detectProcessesList(context.Background(), m.projectDir, m.sandbox)
		m.dashboardItems = buildDashboardItems(m.processes)
	default:
		runes := []rune(key.Key().Text)
		if len(runes) > 0 {
			m.sandboxInput = append(m.sandboxInput, make([]rune, len(runes))...)
			copy(
				m.sandboxInput[m.sandboxInputCursor+len(runes):],
				m.sandboxInput[m.sandboxInputCursor:],
			)
			copy(m.sandboxInput[m.sandboxInputCursor:], runes)
			m.sandboxInputCursor += len(runes)
		}
	}
	return m, nil
}

func (m herdrDashboardModel) updateProcessInput(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.processMode = false
		m.processInput = nil
		m.processInputCursor = 0
		m.err = ""
		if len(m.submenuStack) > 0 {
			last := m.submenuStack[len(m.submenuStack)-1]
			if last.title == "Processes…" {
				m.submenuStack = m.submenuStack[:len(m.submenuStack)-1]
				m.submenu = last.items
				m.submenuTitle = last.title
				m.cursor = last.cursor
			}
		}
		return m, nil
	case "left":
		if m.processInputCursor > 0 {
			m.processInputCursor--
		}
	case "right":
		if m.processInputCursor < len(m.processInput) {
			m.processInputCursor++
		}
	case "home", "ctrl+a":
		m.processInputCursor = 0
	case "end", "ctrl+e":
		m.processInputCursor = len(m.processInput)
	case "backspace", "ctrl+h":
		if m.processInputCursor > 0 {
			m.processInput = append(
				m.processInput[:m.processInputCursor-1],
				m.processInput[m.processInputCursor:]...,
			)
			m.processInputCursor--
		}
	case "delete", "ctrl+d":
		if m.processInputCursor < len(m.processInput) {
			m.processInput = append(
				m.processInput[:m.processInputCursor],
				m.processInput[m.processInputCursor+1:]...,
			)
		}
	case "enter":
		name := strings.TrimSpace(string(m.processInput))
		if name == "" {
			m.err = "enter a process name"
			return m, nil
		}
		base := m.processBase
		if len(base) == 0 {
			base = []string{"processes"}
		}
		m.chosen = append(append([]string(nil), base...), m.processAction, name)
		if m.sandbox != "" && commandSupportsSandbox(m.chosen) {
			m.chosen = append(m.chosen, "--sandbox", m.sandbox)
		}
		m.processMode = false
		m.err = ""
		return m, tea.Quit
	default:
		runes := []rune(key.Key().Text)
		if len(runes) > 0 {
			m.processInput = append(m.processInput, make([]rune, len(runes))...)
			copy(
				m.processInput[m.processInputCursor+len(runes):],
				m.processInput[m.processInputCursor:],
			)
			copy(m.processInput[m.processInputCursor:], runes)
			m.processInputCursor += len(runes)
			m.err = ""
		}
	}
	return m, nil
}

func (m herdrDashboardModel) updateCommand(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.commandMode = false
		m.input = nil
		m.inputCursor = 0
		m.err = ""
		return m, nil
	case "left":
		if m.inputCursor > 0 {
			m.inputCursor--
		}
	case "right":
		if m.inputCursor < len(m.input) {
			m.inputCursor++
		}
	case "home", "ctrl+a":
		m.inputCursor = 0
	case "end", "ctrl+e":
		m.inputCursor = len(m.input)
	case "backspace", "ctrl+h":
		if m.inputCursor > 0 {
			m.input = append(m.input[:m.inputCursor-1], m.input[m.inputCursor:]...)
			m.inputCursor--
		}
	case "delete", "ctrl+d":
		if m.inputCursor < len(m.input) {
			m.input = append(m.input[:m.inputCursor], m.input[m.inputCursor+1:]...)
		}
	case "enter":
		args, err := parseCommandLine(string(m.input))
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		if len(args) == 0 {
			m.err = "enter a Mezha command"
			return m, nil
		}
		if args[0] == "mezha" {
			args = args[1:]
		}
		if len(args) == 0 {
			m.err = "enter a Mezha command"
			return m, nil
		}
		m.chosen = args
		return m, tea.Quit
	default:
		runes := []rune(key.Key().Text)
		if len(runes) > 0 {
			m.input = append(m.input, make([]rune, len(runes))...)
			copy(m.input[m.inputCursor+len(runes):], m.input[m.inputCursor:])
			copy(m.input[m.inputCursor:], runes)
			m.inputCursor += len(runes)
			m.err = ""
		}
	}
	return m, nil
}

func (m herdrDashboardModel) View() tea.View {
	var view strings.Builder
	if m.forceInitMode {
		view.WriteString(
			"This repository already has a Mezha configuration.\n" +
				"Reinitialize it with --force? [y/N]\n\n" +
				"y overwrite  •  n/enter cancel  •  esc back\n",
		)
		result := tea.NewView(view.String())
		result.AltScreen = true
		return result
	}
	if m.settingsMode {
		view.WriteString("Choose a configuration to edit\n")
		if m.settingsNeedInit {
			view.WriteString("No Mezha configuration found; Mezha will initialize it first.\n")
		}
		view.WriteString("\n")
		targetWidth := max(m.width-2, 20)
		for index, setting := range m.settings {
			cursor := "  "
			if index == m.settingCursor {
				cursor = "> "
			}
			digit := ""
			if index < 10 {
				digit = dashboardDigitLabel(index)
			}
			left := fmt.Sprintf("%s%-8s %s", cursor, setting.label, setting.path)
			rightWidth := lipgloss.Width(digit)
			avail := targetWidth - rightWidth - 2
			if lipgloss.Width(left) > avail && avail > 3 {
				left = truncateDashboardText(left, avail)
			}
			padding := targetWidth - lipgloss.Width(left) - rightWidth
			if padding < 1 {
				padding = 1
			}
			fmt.Fprintf(&view, "%s%s%s\n", left, strings.Repeat(" ", padding), digit)
		}
		view.WriteString("\ndigit edit  •  ↑/↓ or j/k move  •  enter edit  •  esc back\n")
		result := tea.NewView(view.String())
		result.AltScreen = true
		return result
	}
	if m.sandboxMode {
		view.WriteString("Choose a sandbox for dashboard actions\n\n")
		for index, sandbox := range m.sandboxes {
			cursor := "  "
			if index == m.sandboxCursor {
				cursor = "> "
			}
			label := sandbox
			if sandbox == "" {
				label = "Configured default"
			} else if m.syncedSandboxes[sandbox] {
				label += "  (synced with this repo)"
			}
			fmt.Fprintf(&view, "%s%s\n", cursor, label)
		}
		cursor := "  "
		if m.sandboxCursor == len(m.sandboxes) {
			cursor = "> "
		}
		fmt.Fprintf(&view, "%sCustom sandbox…\n", cursor)
		view.WriteString("\n↑/↓ or j/k move  •  enter select  •  esc back\n")
		result := tea.NewView(view.String())
		result.AltScreen = true
		return result
	}
	if m.sandboxCustomMode {
		view.WriteString("Enter a sandbox name\n\n  ")
		before := string(m.sandboxInput[:m.sandboxInputCursor])
		after := string(m.sandboxInput[m.sandboxInputCursor:])
		fmt.Fprintf(&view, "%s█%s\n", before, after)
		view.WriteString("\nenter select  •  esc back\n")
		result := tea.NewView(view.String())
		result.AltScreen = true
		return result
	}
	if m.processMode {
		fmt.Fprintf(&view, "Enter process name to %s\n\n  ", m.processAction)
		before := string(m.processInput[:m.processInputCursor])
		after := string(m.processInput[m.processInputCursor:])
		fmt.Fprintf(&view, "%s█%s\n", before, after)
		if m.err != "" {
			fmt.Fprintf(&view, "\n%s\n", m.err)
		}
		view.WriteString("\nenter run  •  esc back\n")
		result := tea.NewView(view.String())
		result.AltScreen = true
		return result
	}
	if m.commandMode {
		view.WriteString("Run any Mezha CLI command\n\n  mezha ")
		before := string(m.input[:m.inputCursor])
		after := string(m.input[m.inputCursor:])
		fmt.Fprintf(&view, "%s█%s\n", before, after)
		if m.err != "" {
			fmt.Fprintf(&view, "\n%s\n", m.err)
		}
		view.WriteString("\nenter run  •  esc back\n")
		result := tea.NewView(view.String())
		result.AltScreen = true
		return result
	}
	w := m.width
	if w <= 0 {
		w = 80
	}
	sandbox := "Configured default"
	if m.sandbox != "" {
		sandbox = m.sandbox
	}
	section := "Mezha"
	if m.submenuTitle != "" {
		section = strings.TrimSuffix(m.submenuTitle, "…")
	}
	view.WriteString(dashboardTitleStyle.Width(w).Render(section))
	view.WriteString("\n\n")

	detail := dashboardDescStyle.Render("Sandbox  ") + dashboardNameStyle.Render(sandbox)
	if m.filterMode || len(m.filter) > 0 {
		detail += "\n" + dashboardPromptStyle.Render("❯ ") + string(m.filter) + "█"
	}
	if m.err != "" {
		detail += "\n" + dashboardErrorStyle.Render(truncateDashboardText(m.err, w-6))
	}
	view.WriteString(dashboardDetailStyle.Width(max(w-2, 20)).Render(detail))
	view.WriteString("\n\n")

	items := m.activeDashboardItems()
	matches := m.filteredDashboardItems()
	targetWidth := max(w-2, 20)
	for position, index := range matches {
		item := items[index]
		selected := position == m.cursor
		prefix := "  "
		if selected {
			prefix = dashboardBarStyle.Render("▌ ")
		}
		nameStyle := dashboardNameStyle
		if selected {
			nameStyle = dashboardNameSelStyle
		}

		digit := ""
		if position < 10 {
			digit = dashboardDigitLabel(position)
		}
		right := ""
		if digit != "" {
			right = nameStyle.Render(digit)
		}

		title := nameStyle.Render(item.title)
		left := prefix + title
		if item.description != "" {
			rightWidth := lipgloss.Width(right)
			prefixWidth := lipgloss.Width(prefix)
			titleWidth := lipgloss.Width(title)
			avail := targetWidth - prefixWidth - titleWidth - 4 - rightWidth
			if avail > 3 {
				desc := truncateDashboardText(item.description, avail)
				left += "  " + dashboardDescStyle.Render(desc)
			}
		}

		leftWidth := lipgloss.Width(left)
		rightWidth := lipgloss.Width(right)
		padding := targetWidth - leftWidth - rightWidth
		if padding < 1 {
			padding = 1
		}
		view.WriteString(left + strings.Repeat(" ", padding) + right + "\n")
	}
	if len(matches) == 0 {
		view.WriteString(dashboardDescStyle.Render("  No matching actions."))
		view.WriteString("\n")
	}
	view.WriteString("\n")
	if m.filterMode {
		view.WriteString(dashboardFooterStyle.Render("↑/↓ move · enter select · esc clear filter"))
	} else {
		escape := "esc close"
		if len(m.submenu) > 0 {
			escape = "esc back"
		}
		view.WriteString(
			dashboardFooterStyle.Render(
				"↑/↓ move · enter select · type filter · space sandbox · " + escape,
			),
		)
	}
	result := tea.NewView(view.String())
	result.AltScreen = true
	return result
}

func restoreSubmenuForCommand(model *herdrDashboardModel, command []string) {
	if len(command) == 0 {
		return
	}
	targetTitle := ""
	if command[0] == "processes" ||
		(len(command) > 1 && command[0] == "sandbox" && command[1] == "processes") {
		targetTitle = "Processes…"
	} else {
		switch command[0] {
		case "sandbox":
			targetTitle = "Sandbox…"
		case "sync":
			targetTitle = "Sync…"
		case "volume":
			targetTitle = "Volume…"
		case "image":
			targetTitle = "Image…"
		}
	}
	if targetTitle == "" {
		return
	}
	for i, item := range model.dashboardItems {
		if item.title == targetTitle {
			model.submenuStack = []herdrSubmenuState{
				{
					items:  nil,
					title:  "",
					cursor: i,
				},
			}
			model.submenu = item.children
			model.submenuTitle = item.title
			model.cursor = 0
			model.filter = nil
			model.filterMode = false
			return
		}
	}
}

func runHerdrDashboard(ctx context.Context, _ *cli.Command) error {
	projectDir, err := herdrProjectDir()
	if err != nil {
		return err
	}
	repoRoot, err := findRepoRoot(projectDir)
	if err != nil {
		return err
	}
	model := herdrDashboardModel{}
	var lastCommand []string
	for {
		if err := refreshHerdrDashboard(ctx, projectDir, &model); err != nil {
			return err
		}
		if len(lastCommand) > 0 {
			restoreSubmenuForCommand(&model, lastCommand)
		}
		result, err := tea.NewProgram(model).Run()
		if err != nil {
			return fmt.Errorf("run Mezha dashboard: %w", err)
		}
		var ok bool
		model, ok = result.(herdrDashboardModel)
		if !ok {
			return nil
		}
		chosenSetting := model.chosenSetting
		settingsNeedInit := model.settingsNeedInit
		command := append([]string(nil), model.chosen...)
		lastCommand = append([]string(nil), model.chosen...)
		model.chosenSetting = ""
		model.chosen = nil
		model.settingsMode = false
		model.forceInitMode = false
		model.commandMode = false
		model.processMode = false
		model.processAction = ""
		model.processBase = nil
		model.processInput = nil
		model.processInputCursor = 0
		model.submenu = nil
		model.submenuStack = nil
		model.submenuTitle = ""
		model.cursor = 0
		model.filter = nil
		model.filterMode = false
		if chosenSetting == "" && len(command) == 0 {
			return nil
		}
		var actionErr error
		if chosenSetting != "" {
			if settingsNeedInit {
				actionErr = Init(ctx, RepoContext{RepoRoot: repoRoot}, false)
				if actionErr != nil {
					actionErr = fmt.Errorf("initialize Mezha settings: %w", actionErr)
				}
			}
			if actionErr == nil {
				actionErr = launchHerdrSettingsEditor(ctx, repoRoot, chosenSetting)
			}
		} else {
			actionErr = launchHerdrDashboardCommand(ctx, command)
			// Pane operations take over the interaction.
			if actionErr == nil && dashboardCommandUsesPane(command) {
				return nil
			}
		}
		if actionErr != nil {
			model.err = actionErr.Error()
		} else {
			model.err = ""
		}
	}
}

func refreshHerdrDashboard(
	ctx context.Context,
	projectDir string,
	model *herdrDashboardModel,
) error {
	sandboxes, syncedSandboxes, err := listHerdrDashboardSandboxes(ctx, projectDir)
	if err != nil {
		return err
	}
	repoInitialized, err := herdrRepoInitialized(projectDir)
	if err != nil {
		return err
	}
	settings, settingsNeedInit, err := herdrDashboardSettings(projectDir)
	if err != nil {
		return err
	}
	processes := detectProcessesList(ctx, projectDir, model.sandbox)
	model.sandboxes = sandboxes
	model.syncedSandboxes = syncedSandboxes
	model.repoInitialized = repoInitialized
	model.settings = settings
	model.settingsNeedInit = settingsNeedInit
	model.processes = processes
	model.projectDir = projectDir
	model.dashboardItems = buildDashboardItems(processes)
	return nil
}

func herdrDashboardSettings(projectDir string) ([]herdrDashboardSetting, bool, error) {
	repoRoot, err := findRepoRoot(projectDir)
	if err != nil {
		return nil, false, err
	}
	mezhaPath := filepath.Join(repoRoot, "mezha.toml")
	extensionsDir := resolveExtensionsDir(repoRoot)
	settings := make([]herdrDashboardSetting, 0, 4)
	if _, configPath, err := LoadConfig(repoRoot); err != nil {
		return nil, false, fmt.Errorf("load Mezha settings: %w", err)
	} else if configPath != "" {
		settings = append(settings, herdrDashboardSetting{label: "Mezha", path: configPath})
	}
	if info, err := os.Stat(extensionsDir); err == nil && info.IsDir() {
		_ = filepath.Walk(extensionsDir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if strings.HasSuffix(info.Name(), ".nix") {
				rel, _ := filepath.Rel(extensionsDir, p)
				settings = append(settings, herdrDashboardSetting{label: rel, path: p})
			}
			return nil
		})
	}
	if len(settings) > 0 {
		return settings, false, nil
	}
	return []herdrDashboardSetting{
		{label: "Mezha", path: mezhaPath},
		{label: "extension", path: filepath.Join(extensionsDir, "sample", "devenv.nix")},
	}, true, nil
}

func launchHerdrSettingsEditor(ctx context.Context, repoRoot, path string) error {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		editor = "vi"
	}
	args, err := parseCommandLine(editor)
	if err != nil {
		return fmt.Errorf("parse editor command: %w", err)
	}
	if len(args) == 0 {
		return errors.New("editor command is empty")
	}
	child := exec.CommandContext(ctx, args[0], append(args[1:], path)...)
	child.Dir = repoRoot
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Run(); err != nil {
		return fmt.Errorf("open %s in editor: %w", path, err)
	}
	return nil
}

func herdrRepoInitialized(projectDir string) (bool, error) {
	repoRoot, err := findRepoRoot(projectDir)
	if err != nil {
		return false, err
	}
	if config, _, err := LoadConfig(repoRoot); err != nil {
		return false, fmt.Errorf("load Mezha configuration: %w", err)
	} else if config != nil {
		return true, nil
	}
	paths := []string{
		filepath.Join(resolveExtensionsDir(repoRoot), "sample", "devenv.nix"),
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, fmt.Errorf("inspect Mezha configuration: %w", err)
		}
	}
	return false, nil
}

func listHerdrDashboardSandboxes(
	ctx context.Context,
	projectDir string,
) ([]string, map[string]bool, error) {
	machines, err := listHerdrMachines(ctx)
	if err != nil {
		return nil, nil, err
	}
	seen := make(map[string]struct{})
	synced := make(map[string]bool)
	var sandboxes []string
	repo, _ := git.PlainOpenWithOptions(projectDir, &git.PlainOpenOptions{DetectDotGit: true})
	for _, machine := range machines {
		if !strings.HasPrefix(machine.Target, "mezha-sandbox-") {
			continue
		}
		name := strings.TrimSpace(machine.Label)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		sandboxes = append(sandboxes, name)
		if repo != nil {
			if rem, err := repo.Remote(name); err == nil && len(rem.Config().URLs) > 0 {
				host := sandboxSSHHostAlias(name)
				synced[name] = strings.Contains(rem.Config().URLs[0], "@"+host+"/")
			}
		}
	}
	sort.Strings(sandboxes)
	return append([]string{""}, sandboxes...), synced, nil
}

func dashboardCommandUsesPane(command []string) bool {
	return len(command) > 0 && (command[0] == "run" ||
		(len(command) > 1 && command[0] == "sandbox" && command[1] == "destroy"))
}

func launchHerdrDashboardCommand(ctx context.Context, command []string) error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve Mezha executable: %w", err)
	}
	args := []string{"herdr"}
	if dashboardCommandUsesPane(command) {
		operation := command[0]
		paneCommand := command[1:]
		switch operation {
		case "run":
			operation = "shell"
		case "sandbox":
			operation = "destroy"
			paneCommand = command[2:]
		}
		args = append(args, "dispatch", operation, "--")
		args = append(args, paneCommand...)
	} else {
		// Keep command output visible until it is acknowledged.
		args = append(args, "execute", "--wait", "--")
		args = append(args, command...)
	}
	child := exec.CommandContext(ctx, binary, args...)
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Run(); err != nil {
		return fmt.Errorf("launch Mezha %s from dashboard: %w", strings.Join(command, " "), err)
	}
	return nil
}

func parseCommandLine(line string) ([]string, error) {
	var args []string
	var word strings.Builder
	var quote rune
	escaped := false
	started := false
	flush := func() {
		args = append(args, word.String())
		word.Reset()
		started = false
	}
	for _, char := range line {
		if escaped {
			word.WriteRune(char)
			escaped = false
			started = true
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				word.WriteRune(char)
			}
			started = true
			continue
		}
		switch {
		case char == '\'' || char == '"':
			quote = char
			started = true
		case unicode.IsSpace(char):
			if started {
				flush()
			}
		default:
			word.WriteRune(char)
			started = true
		}
	}
	if escaped {
		return nil, errors.New("command ends with an incomplete escape")
	}
	if quote != 0 {
		return nil, errors.New("command contains an unterminated quote")
	}
	if started {
		flush()
	}
	return args, nil
}
