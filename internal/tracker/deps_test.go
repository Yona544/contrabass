package tracker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDependencies(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "blocked by with markdown bold",
			body: "**Blocked by:** #2 (Go scaffold), #4 (DB schema)",
			want: []string{"2", "4"},
		},
		{
			name: "blocked by single issue",
			body: "Blocked by: #15\nBlocks: #17",
			want: []string{"15"},
		},
		{
			name: "depends on with commas",
			body: "Depends on: #3, #5, #7",
			want: []string{"3", "5", "7"},
		},
		{
			name: "depends-on with hyphen",
			body: "Depends-on: #8, #9",
			want: []string{"8", "9"},
		},
		{
			name: "requires single issue",
			body: "Requires: #10",
			want: []string{"10"},
		},
		{
			name: "no dependencies",
			body: "No dependencies here",
			want: []string{},
		},
		{
			name: "empty body",
			body: "",
			want: []string{},
		},
		{
			name: "multiple dependency lines",
			body: "Blocked by: #1\nDepends on: #2\nRequires: #3",
			want: []string{"1", "2", "3"},
		},
		{
			name: "deduplicates across lines",
			body: "Blocked by: #5\nDepends on: #5, #6",
			want: []string{"5", "6"},
		},
		{
			name: "deduplicates on same line",
			body: "Depends on: #5, #5, #6",
			want: []string{"5", "6"},
		},
		{
			name: "case insensitive",
			body: "BLOCKED BY: #11\nDEPENDS ON: #12\nREQUIRES: #13",
			want: []string{"11", "12", "13"},
		},
		{
			name: "markdown checklist dependency line",
			body: "- [ ] Blocked by: #21 and #22 before starting",
			want: []string{"21", "22"},
		},
		{
			name: "ignores later issue references",
			body: "Blocked by: #30\nLater mention #31 is not a dependency",
			want: []string{"30"},
		},
		{
			name: "ignores non dependency issue lines",
			body: "Blocks: #40",
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDependencies(tt.body)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseBlockedBy(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []int
	}{
		{
			name: "blocked by with markdown",
			body: "**Blocked by:** #2 (Go scaffold), #4 (DB schema)",
			want: []int{2, 4},
		},
		{
			name: "depends on",
			body: "Depends on: #3, #5, #7",
			want: []int{3, 5, 7},
		},
		{
			name: "requires",
			body: "Requires: #10",
			want: []int{10},
		},
		{
			name: "no dependencies",
			body: "No dependencies here",
			want: []int{},
		},
		{
			name: "deduplicates repeated refs",
			body: "Blocked by: #4, #4, #5",
			want: []int{4, 5},
		},
		{
			name: "ignores later issue references",
			body: "Blocked by: #30\nLater mention #31 is not a dependency",
			want: []int{30},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseBlockedBy(tt.body)
			assert.Equal(t, tt.want, got)
		})
	}
}
