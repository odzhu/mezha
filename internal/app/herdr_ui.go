package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	cli "github.com/urfave/cli/v3"
)

type herdrDashboardItem struct {
	operation   string
	title       string
	description string
}

var herdrDashboardItems = []herdrDashboardItem{
	{
		operation:   "run",
		title:       "Open sandbox shell",
		description: "Open an interactive shell in a new tab",
	},
	{
		operation:   "status",
		title:       "Show status",
		description: "Inspect repository synchronization status",
	},
	{
		operation:   "provision",
		title:       "Provision",
		description: "Create and initialize the configured sandbox",
	},
	{operation: "start", title: "Start", description: "Start the sandbox"},
	{operation: "stop", title: "Stop", description: "Stop the sandbox"},
	{
		operation:   "upload",
		title:       "Upload changes",
		description: "Send local dirty changes to the sandbox",
	},
	{
		operation:   "download",
		title:       "Download changes",
		description: "Bring sandbox dirty changes into the workspace",
	},
	{
		operation:   "pull",
		title:       "Pull commits",
		description: "Pull committed changes from the sandbox",
	},
	{
		operation:   "push",
		title:       "Push commits",
		description: "Push committed changes to the sandbox",
	},
	{
		operation:   "destroy",
		title:       "Destroy",
		description: "Confirm and permanently remove the sandbox",
	},
}

type herdrDashboardModel struct {
	cursor int
	width  int
	chosen string
}

func (m herdrDashboardModel) Init() tea.Cmd {
	return nil
}

func (m herdrDashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
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
		case "enter":
			m.chosen = herdrDashboardItems[m.cursor].operation
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m herdrDashboardModel) View() string {
	var view strings.Builder
	view.WriteString("Mezha\n")
	view.WriteString("Manage this workspace's sandbox\n\n")
	for index, item := range herdrDashboardItems {
		cursor := "  "
		if index == m.cursor {
			cursor = "> "
		}
		fmt.Fprintf(&view, "%s%-20s %s\n", cursor, item.title, item.description)
	}
	view.WriteString("\n↑/↓ or j/k move  •  enter select  •  esc close\n")
	return view.String()
}

func runHerdrDashboard(ctx context.Context, _ *cli.Command) error {
	if _, err := herdrProjectDir(); err != nil {
		return err
	}
	program := tea.NewProgram(herdrDashboardModel{}, tea.WithAltScreen())
	result, err := program.Run()
	if err != nil {
		return fmt.Errorf("run Mezha dashboard: %w", err)
	}
	model, ok := result.(herdrDashboardModel)
	if !ok || model.chosen == "" {
		return nil
	}
	return launchHerdrDashboardOperation(ctx, model.chosen)
}

func launchHerdrDashboardOperation(ctx context.Context, operation string) error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve Mezha executable: %w", err)
	}
	args := []string{"herdr"}
	if operation == "run" || operation == "status" || operation == "destroy" {
		args = append(args, "dispatch", operation)
	} else {
		args = append(args, "execute", "--wait", operation)
	}
	child := exec.CommandContext(ctx, binary, args...)
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Run(); err != nil {
		return fmt.Errorf("launch Mezha %s from dashboard: %w", operation, err)
	}
	return nil
}
