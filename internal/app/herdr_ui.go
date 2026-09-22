package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/odzhu/mezha/internal/execx"
	cli "github.com/urfave/cli/v3"
)

type herdrDashboardItem struct {
	args        []string
	title       string
	description string
	custom      bool
	settings    bool
	children    []herdrDashboardItem
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

var herdrLifecycleItems = []herdrDashboardItem{
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
}

var herdrSyncItems = []herdrDashboardItem{
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
}

var herdrDashboardItems = []herdrDashboardItem{
	{
		args:        []string{"sandbox", "list"},
		title:       "List sandboxes",
		description: "Show all local Mezha sandboxes",
	},
	{
		args:        []string{"volume", "list"},
		title:       "List volumes",
		description: "Show persistent Microsandbox volumes",
	},
	{
		args:        []string{"init"},
		title:       "Initialize",
		description: "Create the project Mezha configuration",
	},
	{
		args:        []string{"run", "--herdr"},
		title:       "Open sandbox shell",
		description: "Open an interactive shell in a new tab",
	},
	{
		args:        []string{"sync", "status"},
		title:       "Show status",
		description: "Inspect repository synchronization status",
	},
	{
		args:        []string{"sandbox", "create", "--herdr"},
		title:       "Create sandbox",
		description: "Create and initialize the configured sandbox",
	},
	{
		title:       "Lifecycle…",
		description: "Recreate, start, stop, or destroy the sandbox",
		children:    herdrLifecycleItems,
	},
	{
		title:       "Synchronize…",
		description: "Upload, download, pull, or push changes",
		children:    herdrSyncItems,
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

type herdrDashboardModel struct {
	cursor             int
	chosen             []string
	commandMode        bool
	filterMode         bool
	forceInitMode      bool
	settingsMode       bool
	sandboxMode        bool
	sandboxCustomMode  bool
	input              []rune
	inputCursor        int
	filter             []rune
	settings           []herdrDashboardSetting
	settingCursor      int
	chosenSetting      string
	settingsNeedInit   bool
	submenu            []herdrDashboardItem
	submenuTitle       string
	sandbox            string
	sandboxes          []string
	syncedSandboxes    map[string]bool
	repoInitialized    bool
	sandboxCursor      int
	sandboxInput       []rune
	sandboxInputCursor int
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
		if len(m.submenu) > 0 {
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
		m.submenu = item.children
		m.submenuTitle = item.title
		m.cursor = 0
		m.filter = nil
		m.filterMode = false
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
	if m.sandbox != "" && item.args[0] != "init" {
		m.chosen = append(m.chosen, "--sandbox", m.sandbox)
	}
	return m, tea.Quit
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
		for index, setting := range m.settings {
			cursor := "  "
			if index == m.settingCursor {
				cursor = "> "
			}
			fmt.Fprintf(
				&view,
				"%s[%s] %-8s %s\n",
				cursor,
				dashboardDigitLabel(index),
				setting.label,
				setting.path,
			)
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
	section := "Sandbox"
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
	for position, index := range matches {
		item := items[index]
		selected := position == m.cursor
		if selected {
			view.WriteString(dashboardBarStyle.Render("▌ "))
		} else {
			view.WriteString("  ")
		}
		nameStyle := dashboardNameStyle
		if selected {
			nameStyle = dashboardNameSelStyle
		}
		view.WriteString(nameStyle.Render("[" + dashboardDigitLabel(position) + "] "))
		view.WriteString("   ")
		view.WriteString(nameStyle.Render(item.title))
		if item.description != "" {
			view.WriteString("  ")
			view.WriteString(dashboardDescStyle.Render(item.description))
		}
		view.WriteString("\n")
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
	for {
		if err := refreshHerdrDashboard(ctx, projectDir, &model); err != nil {
			return err
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
		model.chosenSetting = ""
		model.chosen = nil
		model.settingsMode = false
		model.forceInitMode = false
		model.commandMode = false
		model.submenu = nil
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
	model.sandboxes = sandboxes
	model.syncedSandboxes = syncedSandboxes
	model.repoInitialized = repoInitialized
	model.settings = settings
	model.settingsNeedInit = settingsNeedInit
	return nil
}

func herdrDashboardSettings(projectDir string) ([]herdrDashboardSetting, bool, error) {
	repoRoot, err := findRepoRoot(projectDir)
	if err != nil {
		return nil, false, err
	}
	mezhaPath := filepath.Join(repoRoot, "mezha.toml")
	devenvPath := filepath.Join(repoRoot, ".mezha", "devenv.nix")
	settings := make([]herdrDashboardSetting, 0, 2)
	if _, configPath, err := LoadConfig(repoRoot); err != nil {
		return nil, false, fmt.Errorf("load Mezha settings: %w", err)
	} else if configPath != "" {
		settings = append(settings, herdrDashboardSetting{label: "Mezha", path: configPath})
	}
	if _, err := os.Stat(devenvPath); err == nil {
		settings = append(settings, herdrDashboardSetting{label: "devenv", path: devenvPath})
	} else if !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("inspect devenv settings: %w", err)
	}
	if len(settings) > 0 {
		return settings, false, nil
	}
	return []herdrDashboardSetting{
		{label: "Mezha", path: mezhaPath},
		{label: "devenv", path: devenvPath},
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
		filepath.Join(repoRoot, ".mezha", "devenv.nix"),
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
		remoteURL, err := execx.Output(
			ctx,
			"git",
			"-C",
			projectDir,
			"remote",
			"get-url",
			name,
		)
		if err == nil {
			host := sandboxSSHHostAlias(name)
			synced[name] = strings.Contains(string(remoteURL), "@"+host+"/")
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
