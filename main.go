package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	content    string
	textInput  textinput.Model
	err        error
}

func initialModel(fileContent string) model {
	ti := textinput.New()
	ti.Placeholder = "Enter motif (e.g. ATGC)..."
	ti.Focus()
	ti.CharLimit = 20
	ti.Width = 20

	return model{
		content:   fileContent,
		textInput: ti,
		err:       nil,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		}
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m model) View() string {
	matchStyle := lipgloss.NewStyle().Background(lipgloss.Color("5")).Foreground(lipgloss.Color("15")).Bold(true)

	display := m.content
	searchTerm := m.textInput.Value()
	if searchTerm != "" {
		display = strings.ReplaceAll(m.content, searchTerm, matchStyle.Render(searchTerm))
	}

	return fmt.Sprintf(
		"DNA Motif Hunter\n\n%s\n\n%s\n\n(esc to quit)",
		m.textInput.View(),
		display,
	)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <fasta_file>")
		return
	}

	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Printf("Error reading file: %v", err)
		return
	}

	fileStr := string(raw)
	if len(fileStr) > 500 {
		fileStr = fileStr[:500]
	}

	p := tea.NewProgram(initialModel(fileStr))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
