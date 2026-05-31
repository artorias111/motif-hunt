package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// fileLine represents one line from the original FASTA file.
type fileLine struct {
	lineNum  int
	content  string
	isHeader bool
}

// basePos maps one base in a concatenated sequence back to a file line + column.
type basePos struct {
	lineIdx int // index into model.fileLines
	col     int // 0-indexed column within that line
}

// fastaRecord holds a parsed FASTA entry.
type fastaRecord struct {
	name    string
	lineIdx int // index of the ">" header line in fileLines
	bases   string
	posMap  []basePos
}

// seqMatch is one fuzzy-search hit.
type seqMatch struct {
	recIdx int
	start  int // inclusive index into record.bases
	end    int // exclusive
	score  int // edit distance
}

type model struct {
	fileLines  []fileLine
	records    []fastaRecord
	textInput  textinput.Model
	matches    []seqMatch
	scrollTop  int
	termHeight int
	termWidth  int
	maxEdits   int
	lastQuery  string
	lastEdits  int
}

// matchPalette cycles background colors across distinct hits.
var matchPalette = []string{
	"#FF6B6B", // red
	"#FFD93D", // yellow
	"#6BCB77", // green
	"#4D96FF", // blue
	"#FF6BD6", // pink
	"#FF9F43", // orange
}

// ── FASTA parsing ─────────────────────────────────────────────────────────────

func parseFasta(content string) ([]fileLine, []fastaRecord) {
	rawLines := strings.Split(content, "\n")
	var fileLines []fileLine
	var records []fastaRecord
	var cur *fastaRecord

	for i, raw := range rawLines {
		isHeader := strings.HasPrefix(raw, ">")
		fileLines = append(fileLines, fileLine{
			lineNum:  i + 1,
			content:  raw,
			isHeader: isHeader,
		})
		lineIdx := len(fileLines) - 1

		if isHeader {
			if cur != nil {
				records = append(records, *cur)
			}
			cur = &fastaRecord{
				name:    strings.TrimSpace(strings.TrimPrefix(raw, ">")),
				lineIdx: lineIdx,
			}
		} else if cur != nil {
			for col, ch := range raw {
				if ch != ' ' && ch != '\t' && ch != '\r' {
					cur.bases += strings.ToUpper(string(ch))
					cur.posMap = append(cur.posMap, basePos{lineIdx: lineIdx, col: col})
				}
			}
		}
	}
	if cur != nil {
		records = append(records, *cur)
	}
	return fileLines, records
}

// ── Fuzzy matching (Sellers / semi-global DP) ─────────────────────────────────
//
// Finds all substrings of text that match query with at most maxEdits
// substitutions/insertions/deletions. Returns [start, end, score] triples.

func sellersSearch(text, query string, maxEdits int) [][3]int {
	n, m := len(text), len(query)
	if m == 0 || n == 0 {
		return nil
	}

	dp := make([]int, m+1)
	startDP := make([]int, m+1)
	for j := range dp {
		dp[j] = j
	}

	var results [][3]int

	for i := 1; i <= n; i++ {
		newDP := make([]int, m+1)
		newStart := make([]int, m+1)
		newDP[0] = 0
		newStart[0] = i // free gap at start of text

		for j := 1; j <= m; j++ {
			cost := 1
			if text[i-1] == query[j-1] {
				cost = 0
			}
			sub := dp[j-1] + cost
			del := dp[j] + 1
			ins := newDP[j-1] + 1

			if sub <= del && sub <= ins {
				newDP[j] = sub
				newStart[j] = startDP[j-1]
			} else if del <= ins {
				newDP[j] = del
				newStart[j] = startDP[j]
			} else {
				newDP[j] = ins
				newStart[j] = newStart[j-1]
			}
		}

		if newDP[m] <= maxEdits {
			s := newStart[m] - 1
			if s < 0 {
				s = 0
			}
			results = append(results, [3]int{s, i, newDP[m]})
		}

		dp = newDP
		startDP = newStart
	}
	return results
}

// mergeMatches collapses overlapping hits, keeping the one with the best score.
func mergeMatches(raw [][3]int) [][3]int {
	if len(raw) == 0 {
		return nil
	}
	merged := [][3]int{raw[0]}
	for _, hit := range raw[1:] {
		last := &merged[len(merged)-1]
		if hit[0] < last[1] { // overlapping
			if hit[2] < last[2] {
				*last = hit
			} else if hit[1] > last[1] {
				last[1] = hit[1]
			}
		} else {
			merged = append(merged, hit)
		}
	}
	return merged
}

// ── Model methods ─────────────────────────────────────────────────────────────

func (m *model) runSearch() {
	query := strings.ToUpper(strings.TrimSpace(m.textInput.Value()))
	if query == m.lastQuery && m.maxEdits == m.lastEdits {
		return
	}
	m.lastQuery = query
	m.lastEdits = m.maxEdits
	m.matches = nil

	if len(query) < 2 {
		return
	}

	for ri, rec := range m.records {
		for _, hit := range mergeMatches(sellersSearch(rec.bases, query, m.maxEdits)) {
			m.matches = append(m.matches, seqMatch{
				recIdx: ri,
				start:  hit[0],
				end:    hit[1],
				score:  hit[2],
			})
		}
	}
}

// buildHighlightMap returns a map of (lineIdx, col) → match index.
func (m *model) buildHighlightMap() map[[2]int]int {
	hm := make(map[[2]int]int)
	for mi, match := range m.matches {
		rec := m.records[match.recIdx]
		for pos := match.start; pos < match.end && pos < len(rec.posMap); pos++ {
			bp := rec.posMap[pos]
			hm[[2]int{bp.lineIdx, bp.col}] = mi
		}
	}
	return hm
}

func (m model) seqViewHeight() int {
	h := m.termHeight - 5 // title + input + divider + divider + footer
	if h < 4 {
		h = 4
	}
	return h
}

// ── Bubble Tea ────────────────────────────────────────────────────────────────

func initialModel(content string) model {
	ti := textinput.New()
	ti.Placeholder = "Type a motif to search (e.g. ATGC)..."
	ti.Focus()
	ti.CharLimit = 60
	ti.Width = 50

	fileLines, records := parseFasta(content)

	m := model{
		fileLines:  fileLines,
		records:    records,
		textInput:  ti,
		maxEdits:   1,
		termHeight: 40,
		termWidth:  120,
	}
	return m
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
		case "up", "k":
			if m.scrollTop > 0 {
				m.scrollTop--
			}
		case "down", "j":
			max := len(m.fileLines) - m.seqViewHeight()
			if max < 0 {
				max = 0
			}
			if m.scrollTop < max {
				m.scrollTop++
			}
		case "pgup":
			m.scrollTop -= m.seqViewHeight()
			if m.scrollTop < 0 {
				m.scrollTop = 0
			}
		case "pgdn":
			m.scrollTop += m.seqViewHeight()
			max := len(m.fileLines) - m.seqViewHeight()
			if max < 0 {
				max = 0
			}
			if m.scrollTop > max {
				m.scrollTop = max
			}
		case "+", "=":
			if m.maxEdits < 8 {
				m.maxEdits++
				m.lastQuery = "" // force re-search
			}
		case "-":
			if m.maxEdits > 0 {
				m.maxEdits--
				m.lastQuery = ""
			}
		}
	case tea.WindowSizeMsg:
		m.termHeight = msg.Height
		m.termWidth = msg.Width
	}

	m.textInput, cmd = m.textInput.Update(msg)
	m.runSearch()
	return m, cmd
}

func (m model) View() string {
	// ── Styles ──────────────────────────────────────────────────────────────
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	lineNumStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Width(6).Align(lipgloss.Right)
	headerLineStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	divider := dimStyle.Render(strings.Repeat("─", m.termWidth))
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)

	hm := m.buildHighlightMap()

	var sb strings.Builder

	// Title
	sb.WriteString(titleStyle.Render("  ⬡ Motif Hunt") + "\n")

	// Search bar + status
	editsLabel := statusStyle.Render(fmt.Sprintf(" [±%d edits]", m.maxEdits))
	matchInfo := ""
	if m.textInput.Value() != "" {
		if len(m.matches) == 0 {
			matchInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("  no matches")
		} else {
			matchInfo = statusStyle.Render(fmt.Sprintf("  %d hit(s)", len(m.matches)))
		}
	}
	sb.WriteString(m.textInput.View() + editsLabel + matchInfo + "\n")
	sb.WriteString(divider + "\n")

	// Sequence view
	viewH := m.seqViewHeight()
	end := m.scrollTop + viewH
	if end > len(m.fileLines) {
		end = len(m.fileLines)
	}

	for i := m.scrollTop; i < end; i++ {
		fl := m.fileLines[i]
		lineNum := lineNumStyle.Render(fmt.Sprintf("%d", fl.lineNum))

		if fl.isHeader {
			sb.WriteString(lineNum + "  " + headerLineStyle.Render(fl.content) + "\n")
			continue
		}
		if strings.TrimSpace(fl.content) == "" {
			sb.WriteString(lineNum + "\n")
			continue
		}

		// Render content char-by-char with per-match highlight colors.
		var lineContent strings.Builder
		prevMatchIdx := -2

		for col, ch := range fl.content {
			mi, highlighted := hm[[2]int{i, col}]
			if highlighted {
				// Separator between two distinct adjacent matches on the same line.
				if prevMatchIdx >= 0 && prevMatchIdx != mi {
					lineContent.WriteString(sepStyle.Render("│"))
				}
				colorIdx := mi % len(matchPalette)
				style := lipgloss.NewStyle().
					Background(lipgloss.Color(matchPalette[colorIdx])).
					Foreground(lipgloss.Color("#000000")).
					Bold(true)
				lineContent.WriteString(style.Render(string(ch)))
				prevMatchIdx = mi
			} else {
				prevMatchIdx = -1
				lineContent.WriteString(string(ch))
			}
		}

		sb.WriteString(lineNum + "  " + lineContent.String() + "\n")
	}

	// Footer
	sb.WriteString(divider + "\n")
	sb.WriteString(dimStyle.Render(" ↑/↓ k/j scroll  PgUp/PgDn  +/− fuzzy tolerance  esc quit"))

	return sb.String()
}

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: motif-hunt <fasta_file>")
		os.Exit(1)
	}

	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(
		initialModel(string(raw)),
		tea.WithAltScreen(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
