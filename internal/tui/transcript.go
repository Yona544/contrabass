package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// defaultTranscriptSize is the maximum number of transcript fragments
// retained per issue.
const defaultTranscriptSize = 200

// TranscriptBuffer is a fixed-size ring buffer of agent transcript text
// fragments. It keeps the most recent maxSize fragments, silently dropping
// older ones.
type TranscriptBuffer struct {
	fragments []string
	head      int // next write position
	count     int
	maxSize   int
}

// NewTranscriptBuffer creates a transcript buffer with the given capacity.
func NewTranscriptBuffer(maxSize int) *TranscriptBuffer {
	if maxSize <= 0 {
		maxSize = defaultTranscriptSize
	}
	return &TranscriptBuffer{
		fragments: make([]string, maxSize),
		maxSize:   maxSize,
	}
}

// Push appends a fragment, overwriting the oldest if full.
func (b *TranscriptBuffer) Push(text string) {
	b.fragments[b.head] = text
	b.head = (b.head + 1) % b.maxSize
	if b.count < b.maxSize {
		b.count++
	}
}

// Fragments returns all stored fragments in chronological order (oldest first).
func (b *TranscriptBuffer) Fragments() []string {
	if b.count == 0 {
		return nil
	}
	result := make([]string, b.count)
	start := (b.head - b.count + b.maxSize) % b.maxSize
	for i := 0; i < b.count; i++ {
		result[i] = b.fragments[(start+i)%b.maxSize]
	}
	return result
}

// Len returns the number of stored fragments.
func (b *TranscriptBuffer) Len() int {
	return b.count
}

// transcriptPlaceholder is shown when the selected issue has no transcript yet.
const transcriptPlaceholder = "Waiting for transcript..."

// renderTranscriptPane renders a bordered pane with the live transcript for
// one issue. Fragments are joined, wrapped to the pane width, and tail-trimmed
// to maxBodyLines so the newest output stays visible.
func renderTranscriptPane(width, maxBodyLines int, issueID string, fragments []string) string {
	if width <= 0 {
		width = 80
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	dimStyle := lipgloss.NewStyle().Faint(true)
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))

	innerWidth := width - 8
	if innerWidth < 10 {
		innerWidth = 10
	}

	var body string
	if len(fragments) == 0 {
		body = dimStyle.Render(transcriptPlaceholder)
	} else {
		wrapped := textStyle.Width(innerWidth).Render(strings.Join(fragments, ""))
		lines := strings.Split(wrapped, "\n")
		if maxBodyLines > 0 && len(lines) > maxBodyLines {
			lines = lines[len(lines)-maxBodyLines:]
		}
		body = strings.Join(lines, "\n")
	}

	pane := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(width - 6).
		Render(titleStyle.Render(fmt.Sprintf("TRANSCRIPT  %s", issueID)) + "\n" + body)

	return lipgloss.NewStyle().PaddingLeft(2).Render(pane)
}
