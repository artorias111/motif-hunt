package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E2E2")).Background(lipgloss.Color("#3C3C3C")).Padding(0, 1).Bold(true)
	headerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D7D7D")).Italic(true)
	highlightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Background(lipgloss.Color("#300000")).Bold(true)
	coordStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#4A4A4A")).MarginRight(1)
)

type sequenceBlock struct {
	text       string
	isHeader   bool
	startCoord int
}

type fastaMsg []sequenceBlock

type model struct {
	blocks    []sequenceBlock
	textInput textinput.Model
	viewport  viewport.Model
	ready     bool
}

func readFasta(path string) tea.Cmd {
	return func() tea.Msg {
		file, err := os.Open(path)
		if err != nil {
			return fastaMsg{}
		}
		defer file.Close()

		var blocks []sequenceBlock
		scanner := bufio.NewScanner(file)
		currentCoord := 1

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			if strings.HasPrefix(line, ">") {
				blocks = append(blocks, sequenceBlock{text: line, isHeader: true})
				currentCoord = 1 // Reset coordinate for new contig
			} else {
				blocks = append(blocks, sequenceBlock{
					text:       strings.ToUpper(line),
					isHeader:   false,
					startCoord: currentCoord,
				})
				currentCoord += len(line)
			}
		}
		return fastaMsg(blocks)
	}
}

func (m *model) updateViewportContent() {
	searchTerm := strings.ToUpper(m.textInput.Value())
	var viewBuilder strings.Builder

	for _, block := range m.blocks {
		if block.isHeader {
			viewBuilder.WriteString("\n" + headerStyle.Render(block.text) + "\n")
			continue
		}

		displayLine := block.text
		if searchTerm != "" && len(searchTerm) >= 2 {
			displayLine = strings.ReplaceAll(displayLine, searchTerm, highlightStyle.Render(searchTerm))
		}

		// Prepend the coordinate to the DNA line
		coordStr := fmt.Sprintf("%6d ", block.startCoord)
		viewBuilder.WriteString(coordStyle.Render(coordStr) + displayLine + "\n")
	}

	m.viewport.SetContent(viewBuilder.String())
}

func (m model) Init() tea.Cmd {
	filename := "sequence.fasta"
	if len(os.Args) > 1 {
		filename = os.Args[1]
	}
	return tea.Batch(textinput.Blink, readFasta(filename))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		}
	case fastaMsg:
		m.blocks = msg
		if m.ready {
			m.updateViewportContent()
		}
	case tea.WindowSizeMsg:
		headerHeight, footerHeight := 7, 2
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-(headerHeight+footerHeight))
			m.viewport.YPosition = headerHeight
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - (headerHeight + footerHeight)
		}
		m.updateViewportContent()
	}

	m.textInput, cmd = m.textInput.Update(msg)
	cmds = append(cmds, cmd)

	if m.textInput.Focused() {
		m.updateViewportContent()
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "\n  Initializing Hunter..."
	}

	header := fmt.Sprintf("%s\n\n%s\n%s\n", 
		titleStyle.Render("DNA Motif Hunter"), 
		m.textInput.View(), 
		lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Render(" (arrows to scroll • q to quit)"))

	return fmt.Sprintf("%s\n%s\n", header, m.viewport.View())
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "Search Motif..."
	ti.Focus()
	return model{textInput: ti}
}
