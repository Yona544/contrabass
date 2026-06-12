package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/tracker"
	"github.com/junhoyeo/contrabass/internal/types"
)

type prCall struct {
	dir  string
	name string
	args []string
}

type fakePRExecutor struct {
	calls     []prCall
	responses map[string]struct {
		output string
		err    error
	}
}

func newFakePRExecutor() *fakePRExecutor {
	return &fakePRExecutor{responses: map[string]struct {
		output string
		err    error
	}{}}
}

func (f *fakePRExecutor) respond(prefix, output string, err error) {
	f.responses[prefix] = struct {
		output string
		err    error
	}{output: output, err: err}
}

func (f *fakePRExecutor) exec(_ context.Context, dir, name string, args ...string) (string, error) {
	f.calls = append(f.calls, prCall{dir: dir, name: name, args: args})
	key := name + " " + strings.Join(args, " ")
	for prefix, response := range f.responses {
		if strings.HasPrefix(key, prefix) {
			return response.output, response.err
		}
	}
	return "", nil
}

func (f *fakePRExecutor) commandLines() []string {
	lines := make([]string, 0, len(f.calls))
	for _, call := range f.calls {
		lines = append(lines, call.name+" "+strings.Join(call.args, " "))
	}
	return lines
}

func newPROrchestrator(t *testing.T, exec prExecutor, cfg PullRequestConfig) (*Orchestrator, *tracker.MockTracker) {
	t.Helper()

	mt := tracker.NewMockTracker()
	orch := NewOrchestrator(mt, nil, nil, nil, nil)
	orch.SetPullRequestConfig(cfg)
	orch.prExec = exec
	return orch, mt
}

func prTestEntry() (*runEntry, types.RunAttempt) {
	entry := &runEntry{
		issue: types.Issue{
			ID:         "CB-12",
			Identifier: "CB-12",
			Title:      "Add retry metrics",
			URL:        "https://example.test/CB-12",
			BranchName: "symphony/cb-12",
		},
		workspace: "/tmp/workspaces/CB-12",
	}
	attempt := types.RunAttempt{Attempt: 1, TokensIn: 10, TokensOut: 20}
	return entry, attempt
}

func TestMaybeOpenPullRequestDisabledDoesNothing(t *testing.T) {
	fake := newFakePRExecutor()
	orch, _ := newPROrchestrator(t, fake.exec, PullRequestConfig{Enabled: false})
	entry, attempt := prTestEntry()

	orch.maybeOpenPullRequest(context.Background(), entry, attempt)

	assert.Empty(t, fake.calls)
}

func TestMaybeOpenPullRequestPushesAndCreatesDraft(t *testing.T) {
	fake := newFakePRExecutor()
	fake.respond("gh pr view", "", errors.New("no pull requests found"))
	fake.respond("gh pr create", "https://github.com/example/repo/pull/7", nil)

	orch, mt := newPROrchestrator(t, fake.exec, PullRequestConfig{Enabled: true, Draft: true, Base: "main"})
	entry, attempt := prTestEntry()

	orch.maybeOpenPullRequest(context.Background(), entry, attempt)

	lines := fake.commandLines()
	require.Len(t, lines, 3)
	assert.Equal(t, "git push -u origin symphony/cb-12", lines[0])
	assert.Contains(t, lines[1], "gh pr view symphony/cb-12")
	assert.Contains(t, lines[2], "gh pr create --head symphony/cb-12")
	assert.Contains(t, lines[2], "--title CB-12: Add retry metrics")
	assert.Contains(t, lines[2], "--draft")
	assert.Contains(t, lines[2], "--base main")
	for _, call := range fake.calls {
		assert.Equal(t, "/tmp/workspaces/CB-12", call.dir)
	}

	comments := mt.Comments["CB-12"]
	require.Len(t, comments, 1)
	assert.Contains(t, comments[0], "Pull request opened")
	assert.Contains(t, comments[0], "https://github.com/example/repo/pull/7")
}

func TestMaybeOpenPullRequestReusesExistingPR(t *testing.T) {
	fake := newFakePRExecutor()
	fake.respond("gh pr view", "https://github.com/example/repo/pull/4", nil)

	orch, mt := newPROrchestrator(t, fake.exec, PullRequestConfig{Enabled: true})
	entry, attempt := prTestEntry()

	orch.maybeOpenPullRequest(context.Background(), entry, attempt)

	for _, line := range fake.commandLines() {
		assert.NotContains(t, line, "pr create")
	}
	comments := mt.Comments["CB-12"]
	require.Len(t, comments, 1)
	assert.Contains(t, comments[0], "Pull request updated")
	assert.Contains(t, comments[0], "pull/4")
}

func TestMaybeOpenPullRequestPushFailureIsReported(t *testing.T) {
	fake := newFakePRExecutor()
	fake.respond("git push", "", fmt.Errorf("remote rejected"))

	orch, mt := newPROrchestrator(t, fake.exec, PullRequestConfig{Enabled: true})
	entry, attempt := prTestEntry()

	orch.maybeOpenPullRequest(context.Background(), entry, attempt)

	lines := fake.commandLines()
	require.Len(t, lines, 1)
	comments := mt.Comments["CB-12"]
	require.Len(t, comments, 1)
	assert.Contains(t, comments[0], "Pull request creation failed")
	assert.Contains(t, comments[0], "remote rejected")
}

func TestMaybeOpenPullRequestResolvesBranchFromWorkspace(t *testing.T) {
	fake := newFakePRExecutor()
	fake.respond("git rev-parse --abbrev-ref HEAD", "feature/resolved", nil)
	fake.respond("gh pr view", "", errors.New("no pull requests found"))
	fake.respond("gh pr create", "https://github.com/example/repo/pull/9", nil)

	orch, _ := newPROrchestrator(t, fake.exec, PullRequestConfig{Enabled: true})
	entry, attempt := prTestEntry()
	entry.issue.BranchName = ""

	orch.maybeOpenPullRequest(context.Background(), entry, attempt)

	lines := fake.commandLines()
	require.GreaterOrEqual(t, len(lines), 2)
	assert.Equal(t, "git rev-parse --abbrev-ref HEAD", lines[0])
	assert.Equal(t, "git push -u origin feature/resolved", lines[1])
}

func TestLastNonEmptyLine(t *testing.T) {
	assert.Equal(t, "https://x/pr/1", lastNonEmptyLine("Creating pull request\n\nhttps://x/pr/1\n"))
	assert.Equal(t, "", lastNonEmptyLine("  \n \n"))
}
