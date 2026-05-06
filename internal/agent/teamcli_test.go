package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTeamNameFromOutput(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "omx v0.16 banner with hash suffix",
			in: `[omx:team] worker startup resolution: model=gpt-5.5 thinking_level=medium source=role-default
[omx:team] worker startup resolution: model=gpt-5.5 thinking_level=medium source=role-default
Team started: add-godoc-comments-fo-d3d7562b
tmux target: omx-foo:0
workers: 2
`,
			want: "add-godoc-comments-fo-d3d7562b",
		},
		{
			name: "team name with surrounding whitespace",
			in:   "  Team started:    my-team  \n",
			want: "my-team",
		},
		{
			name: "older CLI prints just 'Team started' without name",
			in:   "Team started\n",
			want: "",
		},
		{
			name: "no banner at all (empty fallback)",
			in:   "some other unrelated output\n",
			want: "",
		},
		{
			name: "blank input",
			in:   "",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTeamNameFromOutput([]byte(tc.in))
			assert.Equal(t, tc.want, got)
		})
	}
}
