package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBackoffEmpty(t *testing.T) {
	b := NewBackoff()
	assert.Equal(t, "", b.View())
}

func TestBackoffWithRows(t *testing.T) {
	rows := []BackoffRow{
		{IssueID: "ISSUE-789", Attempt: 3, RetryIn: "45s", Error: "agent timeout"},
		{IssueID: "ISSUE-012", Attempt: 1, RetryIn: "10s", Error: "rate limited"},
	}
	b := NewBackoff().Update(rows)
	out := b.View()
	assert.Contains(t, out, "ISSUE-789")
	assert.Contains(t, out, "ISSUE-012")
	assert.Contains(t, out, "attempt 3")
	assert.Contains(t, out, "retry in")
}

func TestBackoffContainsError(t *testing.T) {
	rows := []BackoffRow{
		{IssueID: "ERR-1", Attempt: 2, RetryIn: "30s", Error: "server overload"},
	}
	b := NewBackoff().Update(rows)
	out := b.View()
	assert.Contains(t, out, "server overload")
}

func TestBackoffSetWidth(t *testing.T) {
	b := NewBackoff().SetWidth(80)
	rows := []BackoffRow{{IssueID: "W-1", Attempt: 1, RetryIn: "5s", Error: "err"}}
	b = b.Update(rows)
	out := b.View()
	assert.Contains(t, out, "W-1")
}

func TestBackoffViewOmitsErrorWhenWidthLeavesNoRoom(t *testing.T) {
	rows := []BackoffRow{{IssueID: "ERR-1", Attempt: 2, RetryIn: "30s", Error: "server overload"}}
	out := stripANSI(NewBackoff().SetWidth(1).Update(rows).View())

	assert.Contains(t, out, "ERR-1")
	assert.Contains(t, out, "attempt 2")
	assert.Contains(t, out, "retry in 30s")
	assert.Contains(t, out, "error:")
	assert.NotContains(t, out, "server overload")
}

func TestTruncateRunesWithEllipsis(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{name: "zero max", input: "abcdef", max: 0, want: ""},
		{name: "negative max", input: "abcdef", max: -1, want: ""},
		{name: "under limit", input: "abc", max: 5, want: "abc"},
		{name: "exact limit", input: "abc", max: 3, want: "abc"},
		{name: "small max omits ellipsis", input: "abcdef", max: 3, want: "abc"},
		{name: "truncates with ellipsis", input: "abcdef", max: 5, want: "ab..."},
		{name: "unicode counts runes", input: "abçdéf", max: 5, want: "ab..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, truncateRunesWithEllipsis(tt.input, tt.max))
		})
	}
}
