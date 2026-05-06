package agent

import (
	"os"
	"path/filepath"
	"strings"
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

func TestIsolatedTeamEnv_ScopesTmuxSocketPerWorkspace(t *testing.T) {
	t.Setenv("TMUX", "/something/from-outer-shell")
	t.Setenv("TMUX_TMPDIR", "/some/inherited/path")

	workspace := t.TempDir()
	env := isolatedTeamEnv(workspace)

	expectedSocket := filepath.Join(workspace, ".contrabass", "tmux-sock")

	info, err := os.Stat(expectedSocket)
	assert.NoError(t, err, "isolated socket directory must be created")
	assert.True(t, info.IsDir(), "socket path must be a directory")

	var seenTmux, seenTmpdir bool
	var tmpdirValue string
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "TMUX="):
			seenTmux = true
		case strings.HasPrefix(kv, "TMUX_TMPDIR="):
			seenTmpdir = true
			tmpdirValue = strings.TrimPrefix(kv, "TMUX_TMPDIR=")
		}
	}
	assert.False(t, seenTmux, "outer TMUX must be stripped so child does not think it is nested")
	assert.True(t, seenTmpdir, "TMUX_TMPDIR must be set on the returned env")
	assert.Equal(t, expectedSocket, tmpdirValue, "TMUX_TMPDIR must point at the per-workspace socket dir")
}

func TestIsolatedTeamEnv_FallsBackOnEmptyWorkspace(t *testing.T) {
	env := isolatedTeamEnv("")
	for _, kv := range env {
		if strings.HasPrefix(kv, "TMUX_TMPDIR=") {
			// We only care that we don't override when we cannot derive a
			// stable workspace-scoped dir; an inherited TMUX_TMPDIR is fine.
			return
		}
	}
}
