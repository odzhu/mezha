package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	cli "github.com/urfave/cli/v3"
)

type herdrDashboardItem struct {
	args        []string
	title       string
	description string
	custom      bool
}

var herdrDashboardItems = []herdrDashboardItem{
	{
		args:        []string{"run", "--herdr", "true"},
		title:       "Open sandbox shell",
		description: "Open an interactive shell in a new tab",
	},
	{
		args:        []string{"status"},
		title:       "Show status",
		description: "Inspect repository synchronization status",
	},
	{
		args:        []string{"provision", "--herdr", "true"},
		title:       "Provision",
		description: "Create and initialize the configured sandbox",
	},
	{
		args:        []string{"recreate", "--herdr", "true"},
		title:       "Recreate",
		description: "Recreate the sandbox with clean persistent state",
	},
	{args: []string{"start"}, title: "Start", description: "Start the sandbox"},
	{args: []string{"stop"}, title: "Stop", description: "Stop the sandbox"},
	{
		args:        []string{"upload"},
		title:       "Upload changes",
		description: "Send local dirty changes to the sandbox",
	},
	{
		args:        []string{"download"},
		title:       "Download changes",
		description: "Bring sandbox dirty changes into the workspace",
	},
	{
		args:        []string{"pull"},
		title:       "Pull commits",
		description: "Pull committed changes from the sandbox",
	},
	{
		args:        []string{"push"},
		title:       "Push commits",
		description: "Push committed changes to the sandbox",
	},
	{
		args:        []string{"destroy"},
		title:       "Destroy",
		description: "Confirm and permanently remove the sandbox",
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
	sandboxMode        bool
	sandboxCustomMode  bool
	input              []rune
	inputCursor        int
	sandbox            string
	sandboxes          []string
	sandboxCursor      int
	sandboxInput       []rune
	sandboxInputCursor int
	err                string
}

func (m herdrDashboardModel) Init() tea.Cmd {
	return nil
}

func (m herdrDashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if m.commandMode {
		return m.updateCommand(key)
	}
	if m.sandboxCustomMode {
		return m.updateSandboxInput(key)
	}
	if m.sandboxMode {
		return m.updateSandbox(key)
	}
	switch key.String() {
	case "ctrl+c", "esc", "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(herdrDashboardItems)-1 {
			m.cursor++
		}
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(herdrDashboardItems) - 1
	case "s":
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
		item := herdrDashboardItems[m.cursor]
		if item.custom {
			m.commandMode = true
			m.err = ""
			return m, nil
		}
		m.chosen = append([]string(nil), item.args...)
		if m.sandbox != "" {
			m.chosen = append(m.chosen, "--sandbox", m.sandbox)
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m herdrDashboardModel) updateSandbox(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	itemCount := len(m.sandboxes) + 1
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.sandboxMode = false
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
		m.err = ""
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
		m.err = ""
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
	view.WriteString("Mezha\n")
	if m.sandboxMode {
		view.WriteString("Choose a sandbox for dashboard actions\n\n")
		for index, sandbox := range m.sandboxes {
			cursor := "  "
			if index == m.sandboxCursor {
				cursor = "> "
			}
			if sandbox == "" {
				sandbox = "Configured default"
			}
			fmt.Fprintf(&view, "%s%s\n", cursor, sandbox)
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
		view.WriteString("\nenter run  •  esc back  •  ctrl+c close\n")
		result := tea.NewView(view.String())
		result.AltScreen = true
		return result
	}
	sandbox := "configured default"
	if m.sandbox != "" {
		sandbox = m.sandbox
	}
	fmt.Fprintf(&view, "Manage this workspace's sandbox\nSandbox: %s\n\n", sandbox)
	for index, item := range herdrDashboardItems {
		cursor := "  "
		if index == m.cursor {
			cursor = "> "
		}
		fmt.Fprintf(&view, "%s%-20s %s\n", cursor, item.title, item.description)
	}
	view.WriteString("\n↑/↓ or j/k move  •  s sandbox  •  enter select  •  esc close\n")
	result := tea.NewView(view.String())
	result.AltScreen = true
	return result
}

func runHerdrDashboard(ctx context.Context, _ *cli.Command) error {
	if _, err := herdrProjectDir(); err != nil {
		return err
	}
	sandboxes, err := listHerdrDashboardSandboxes(ctx)
	if err != nil {
		return err
	}
	program := tea.NewProgram(herdrDashboardModel{sandboxes: sandboxes})
	result, err := program.Run()
	if err != nil {
		return fmt.Errorf("run Mezha dashboard: %w", err)
	}
	model, ok := result.(herdrDashboardModel)
	if !ok || len(model.chosen) == 0 {
		return nil
	}
	return launchHerdrDashboardCommand(ctx, model.chosen)
}

func listHerdrDashboardSandboxes(ctx context.Context) ([]string, error) {
	machines, err := listHerdrMachines(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
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
	}
	sort.Strings(sandboxes)
	return append([]string{""}, sandboxes...), nil
}

func launchHerdrDashboardCommand(ctx context.Context, command []string) error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve Mezha executable: %w", err)
	}
	args := []string{"herdr"}
	if command[0] == "run" || command[0] == "status" || command[0] == "destroy" {
		args = append(args, "dispatch", command[0], "--")
		args = append(args, command...)
	} else {
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
