# motif-hunt

A terminal UI for fuzzy searching DNA sequences in FASTA files — like `Ctrl+F` for genomes. Type a query and matching regions light up in real time across the full sequence, with approximate (mismatch-tolerant) matching and colour-coded hits.

## Features

- **Real-time fuzzy search** — results update on every keystroke
- **Approximate matching** — find sequences with mismatches (adjustable threshold)
- **Colour-coded hits** — exact matches in green, mismatches in yellow/red; consecutive hits alternate colours so adjacent matches stay visually distinct
- **Preserved line numbers** — original FASTA file line numbers shown in a fixed gutter
- **Scrollable viewport** — mouse wheel and arrow keys; auto-jumps to the first hit on each new query
- **FASTA-aware** — header lines (`>...`) rendered separately in italic blue

## Installation

### Prerequisites

- [Go](https://go.dev/dl/) 1.21 or later

Verify your Go installation:

```bash
go version
```

### Build from source

```bash
git clone https://github.com/artorias111/motif-hunt.git
cd motif-hunt
go build -o motif-hunt .
```

This produces a single `motif-hunt` binary with no runtime dependencies.

## Usage

```bash
./motif-hunt <fasta-file>
```

Example:

```bash
./motif-hunt genome.fa
```

Or run directly without building:

```bash
go run . genome.fa
```

## Controls

| Key | Action |
|-----|--------|
| Type | Search in real time |
| `+` / `=` | Allow one more mismatch |
| `-` | Require one fewer mismatch |
| `↑` / `↓` | Scroll one line |
| `PgUp` / `PgDn` | Scroll half a page |
| Mouse wheel | Scroll |
| `Ctrl+C` / `Esc` | Quit |

## How matching works

motif-hunt uses a **sliding window Hamming search** — for every window of `len(query)` nucleotides in the genome, it counts substitutions (mismatches). A hit is reported when the count is within the current mismatch budget.

- **0 mismatches** (default) — exact substring match
- **+1 mismatch** — one nucleotide can differ
- and so on, adjusted live with `+` / `-`

Hits do not overlap: once a match is found at position `i`, the next candidate starts at `i + len(query)`.

## FASTA format

Standard FASTA files are supported:

```
>sequence_name optional description
ATGCATGCATGC...
GCTAGCTAGCTA...
>another_sequence
TTTTAAAACCCC...
```

Both uppercase and lowercase nucleotides are accepted (search is case-insensitive).

## Dependencies

All dependencies are managed with Go modules and downloaded automatically on first build.

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/charmbracelet/bubbletea` | v1.3.10 | TUI framework |
| `github.com/charmbracelet/bubbles` | v1.0.0 | Viewport and text input components |
| `github.com/charmbracelet/lipgloss` | v1.1.0 | Terminal styling and colours |

## Development

```bash
# Run with the included test file
go run . test_files/test.fa

# Update dependencies
go get -u ./...
go mod tidy
```
