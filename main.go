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

// ── styles ────────────────────────────────────────────────────────────────────

var (
	lineNumStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	headerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Italic(true)
	statusStyle  = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)
	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// Two alternating highlight palettes (bg, fg) so adjacent hits stay distinct.
	// Index 0 = exact match, 1 = 1 mismatch, etc. — but we also cycle by match index.
	hitPalette = [][2]string{
		{"#00AA55", "#FFFFFF"}, // green  (primary)
		{"#0077CC", "#FFFFFF"}, // blue   (alternate)
		{"#CC7700", "#FFFFFF"}, // amber  (tertiary)
		{"#AA00AA", "#FFFFFF"}, // purple (quaternary)
	}
	mmPalette = [][2]string{
		{"#AACC00", "#000000"}, // yellow-green  (1 mm, primary)
		{"#00AACC", "#000000"}, // cyan           (1 mm, alternate)
		{"#CCAA00", "#000000"}, // gold
		{"#CC00AA", "#000000"}, // pink
	}
	highMMStyle = lipgloss.NewStyle().Background(lipgloss.Color("#CC3300")).Foreground(lipgloss.Color("#FFFFFF"))
)

func hitStyle(matchIdx, dist, maxMM int) lipgloss.Style {
	palette := hitPalette
	if dist > 0 {
		if maxMM >= 2 && dist == maxMM {
			return highMMStyle
		}
		palette = mmPalette
	}
	p := palette[matchIdx%len(palette)]
	return lipgloss.NewStyle().Background(lipgloss.Color(p[0])).Foreground(lipgloss.Color(p[1]))
}

// ── FASTA parsing ─────────────────────────────────────────────────────────────

type fastaLine struct {
	fileLineNum int    // 1-based line number in the source file
	content     string // raw content of this line
	isHeader    bool
}

func parseFasta(filename string) ([]fastaLine, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []fastaLine
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		text := scanner.Text()
		lines = append(lines, fastaLine{
			fileLineNum: lineNum,
			content:     text,
			isHeader:    strings.HasPrefix(text, ">"),
		})
	}
	return lines, scanner.Err()
}

// ── flat sequence index ───────────────────────────────────────────────────────

type charPos struct {
	lineIdx int // index into []fastaLine
	col     int // 0-based byte offset within that line
}

func buildIndex(lines []fastaLine) (flatSeq string, posMap []charPos) {
	var sb strings.Builder
	for i, l := range lines {
		if l.isHeader {
			continue
		}
		upper := strings.ToUpper(l.content)
		for j := range upper {
			sb.WriteByte(upper[j])
			posMap = append(posMap, charPos{i, j})
		}
	}
	return sb.String(), posMap
}

// ── fuzzy search (Hamming / mismatch-based sliding window) ────────────────────

type match struct {
	start    int // position in flatSeq
	end      int // exclusive
	dist     int // number of mismatches
	matchIdx int // ordinal index for colour cycling
}

func search(seq, query string, maxMM int) []match {
	n, q := len(seq), len(query)
	if q == 0 || q > n {
		return nil
	}
	qUp := strings.ToUpper(query)
	var results []match
	idx := 0
	for i := 0; i <= n-q; i++ {
		mm := 0
		for j := 0; j < q; j++ {
			if seq[i+j] != qUp[j] {
				mm++
				if mm > maxMM {
					break
				}
			}
		}
		if mm <= maxMM {
			results = append(results, match{i, i + q, mm, idx})
			idx++
			i += q - 1 // skip past this hit so matches don't overlap
		}
	}
	return results
}

// ── model ─────────────────────────────────────────────────────────────────────

type model struct {
	lines    []fastaLine
	flatSeq  string
	posMap   []charPos
	filename string

	viewport  viewport.Model
	textInput textinput.Model

	query   string
	matches []match
	maxMM   int

	width  int
	height int
	ready  bool
	err    error
}

func newModel(filename string, lines []fastaLine) model {
	flatSeq, posMap := buildIndex(lines)

	ti := textinput.New()
	ti.Placeholder = "Search sequence (e.g. ATGC)…"
	ti.Focus()
	ti.CharLimit = 200

	return model{
		lines:     lines,
		flatSeq:   flatSeq,
		posMap:    posMap,
		filename:  filename,
		textInput: ti,
		maxMM:     0,
	}
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpH := m.height - 4 // header + input + help
		if vpH < 1 {
			vpH = 1
		}
		if !m.ready {
			m.viewport = viewport.New(m.width, vpH)
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpH
		}
		m.rebuildContent()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "+", "=":
			m.maxMM++
			m.updateSearch()
		case "-":
			if m.maxMM > 0 {
				m.maxMM--
				m.updateSearch()
			}
		default:
			var tiCmd tea.Cmd
			m.textInput, tiCmd = m.textInput.Update(msg)
			cmds = append(cmds, tiCmd)

			newQ := strings.ToUpper(strings.TrimSpace(m.textInput.Value()))
			if newQ != m.query {
				m.query = newQ
				m.updateSearch()
			}
		}
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m *model) updateSearch() {
	m.matches = search(m.flatSeq, m.query, m.maxMM)
	m.rebuildContent()
	// Jump to first match.
	if len(m.matches) > 0 && m.ready {
		firstFlatIdx := m.matches[0].start
		if firstFlatIdx < len(m.posMap) {
			targetLine := m.posMap[firstFlatIdx].lineIdx
			// Count rendered lines up to targetLine.
			rendered := 0
			for i := 0; i < targetLine && i < len(m.lines); i++ {
				rendered++
			}
			// Place the hit near the top with some context.
			offset := rendered - 3
			if offset < 0 {
				offset = 0
			}
			m.viewport.SetYOffset(offset)
		}
	}
}

// buildHighlightMaps builds per-line-index maps of col → (matchIdx, dist).
func (m *model) buildHighlightMaps() map[int]map[int][2]int {
	out := make(map[int]map[int][2]int) // lineIdx → col → {matchIdx, dist}
	for _, mt := range m.matches {
		for pos := mt.start; pos < mt.end; pos++ {
			if pos >= len(m.posMap) {
				break
			}
			cp := m.posMap[pos]
			if out[cp.lineIdx] == nil {
				out[cp.lineIdx] = make(map[int][2]int)
			}
			// Don't overwrite a better (lower dist) highlight.
			if existing, ok := out[cp.lineIdx][cp.col]; !ok || mt.dist < existing[1] {
				out[cp.lineIdx][cp.col] = [2]int{mt.matchIdx, mt.dist}
			}
		}
	}
	return out
}

func (m *model) rebuildContent() {
	if !m.ready {
		return
	}

	hlMaps := m.buildHighlightMaps()
	lineNumW := numWidth(len(m.lines)) + 1

	var sb strings.Builder
	for i, l := range m.lines {
		// Line number column.
		numStr := fmt.Sprintf("%*d ", lineNumW, l.fileLineNum)
		sb.WriteString(lineNumStyle.Render(numStr))

		if l.isHeader {
			sb.WriteString(headerStyle.Render(l.content))
			sb.WriteByte('\n')
			continue
		}

		colMap := hlMaps[i] // may be nil

		if len(colMap) == 0 {
			sb.WriteString(l.content)
			sb.WriteByte('\n')
			continue
		}

		// Render character-by-character, grouping runs of same (matchIdx,dist).
		j := 0
		content := l.content
		for j < len(content) {
			if info, ok := colMap[j]; ok {
				// Collect the run while matchIdx and dist stay the same.
				start := j
				for j < len(content) {
					info2, ok2 := colMap[j]
					if !ok2 || info2 != info {
						break
					}
					j++
				}
				style := hitStyle(info[0], info[1], m.maxMM)
				sb.WriteString(style.Render(content[start:j]))
			} else {
				sb.WriteByte(content[j])
				j++
			}
		}
		sb.WriteByte('\n')
	}

	m.viewport.SetContent(sb.String())
}

func (m model) View() string {
	if !m.ready {
		return "Loading…"
	}
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress Ctrl+C to quit.", m.err)
	}

	// Status bar.
	mmInfo := fmt.Sprintf("mismatches ≤ %d  (+/- to adjust)", m.maxMM)
	hitInfo := "no query"
	if m.query != "" {
		hitInfo = fmt.Sprintf("%d hit(s) for %q", len(m.matches), m.query)
	}
	status := statusStyle.Width(m.width).Render(
		fmt.Sprintf(" motif-hunt  %s  │  %s", m.filename, hitInfo) +
			strings.Repeat(" ", max(0, m.width-len(fmt.Sprintf(" motif-hunt  %s  │  %s  %s", m.filename, hitInfo, mmInfo))-2)) +
			mmInfo + " ",
	)

	help := helpStyle.Render("  ↑/↓ or mouse-wheel to scroll  │  Ctrl+C to quit")

	return lipgloss.JoinVertical(lipgloss.Left,
		status,
		m.viewport.View(),
		m.textInput.View(),
		help,
	)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func numWidth(n int) int {
	if n == 0 {
		return 1
	}
	w := 0
	for n > 0 {
		w++
		n /= 10
	}
	return w
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── entry point ───────────────────────────────────────────────────────────────

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: motif-hunt <fasta-file>")
		os.Exit(1)
	}
	filename := os.Args[1]

	lines, err := parseFasta(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	m := newModel(filename, lines)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
