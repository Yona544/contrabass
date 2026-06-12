package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

func gitReview(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, string(output))
	return string(output)
}

func writeReviewFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

// initReviewRepo builds a real temp git repo with one commit on main.
func initReviewRepo(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()
	gitReview(t, repoDir, "init")
	gitReview(t, repoDir, "config", "user.email", "test@example.com")
	gitReview(t, repoDir, "config", "user.name", "test")
	gitReview(t, repoDir, "config", "commit.gpgsign", "false")

	writeReviewFile(t, repoDir, "README.md", "# review test\n")
	gitReview(t, repoDir, "add", "README.md")
	gitReview(t, repoDir, "commit", "-m", "initial commit")
	gitReview(t, repoDir, "branch", "-M", "main")
	return repoDir
}

// addReviewRunBranch creates an agent run branch with one commit and returns
// to main, mimicking a finished contrabass run.
func addReviewRunBranch(t *testing.T, repoDir, branch string) {
	t.Helper()

	gitReview(t, repoDir, "checkout", "-b", branch)
	writeReviewFile(t, repoDir, "feature.txt", "agent change\n")
	gitReview(t, repoDir, "add", "feature.txt")
	gitReview(t, repoDir, "commit", "-m", "agent: add feature")
	gitReview(t, repoDir, "checkout", "main")
}

func reviewBranchExistsForReal(t *testing.T, repoDir, branch string) bool {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repoDir
	return cmd.Run() == nil
}

func runReviewCmd(t *testing.T, repoDir, stdin string, args ...string) (string, error) {
	t.Helper()

	cmd := newReviewCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(append(args, "--repo", repoDir))
	err := cmd.Execute()
	return out.String(), err
}

func stubReviewGit(t *testing.T, fn func(ctx context.Context, repoPath string, args ...string) (string, error)) {
	t.Helper()

	original := reviewRunGit
	t.Cleanup(func() { reviewRunGit = original })
	reviewRunGit = fn
}

func stubReviewFeedback(t *testing.T, fn func(issueOrBranch, feedback string) error) {
	t.Helper()

	original := reviewFeedback
	t.Cleanup(func() { reviewFeedback = original })
	reviewFeedback = fn
}

// --- branch resolution ---

func TestReviewResolvesExplicitBranch(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	stdout, err := runReviewCmd(t, repoDir, "q\n", "symphony/cb-1")
	require.NoError(t, err)
	assert.Contains(t, stdout, "reviewing symphony/cb-1 against main")
	assert.Contains(t, stdout, "agent: add feature")
	assert.Contains(t, stdout, "feature.txt")
}

func TestReviewDerivesBranchFromIssueID(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	stdout, err := runReviewCmd(t, repoDir, "q\n", "CB-1")
	require.NoError(t, err)
	assert.Contains(t, stdout, "reviewing symphony/cb-1 against main")
}

func TestReviewDerivesBranchFromConfigPrefix(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "task/cb-2")

	cfgPath := filepath.Join(t.TempDir(), "WORKFLOW.md")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`---
model: m
tracker:
  type: internal
workspace:
  branch_prefix: task/
---
Prompt.
`), 0o644))

	stdout, err := runReviewCmd(t, repoDir, "q\n", "CB-2", "--config", cfgPath)
	require.NoError(t, err)
	assert.Contains(t, stdout, "reviewing task/cb-2 against main")
}

func TestReviewMissingBranchFails(t *testing.T) {
	repoDir := initReviewRepo(t)

	_, err := runReviewCmd(t, repoDir, "", "CB-404")
	require.Error(t, err)
	assert.ErrorContains(t, err, `"CB-404"`)
	assert.ErrorContains(t, err, `"symphony/cb-404"`)
}

// --- base resolution ---

func TestReviewBaseFromOriginHead(t *testing.T) {
	stubReviewGit(t, func(_ context.Context, _ string, args ...string) (string, error) {
		switch strings.Join(args, " ") {
		case "symbolic-ref --short refs/remotes/origin/HEAD":
			return "origin/trunk\n", nil
		case "rev-parse --verify --quiet refs/heads/trunk":
			return "", nil
		default:
			return "", errors.New("unexpected git call: " + strings.Join(args, " "))
		}
	})

	base, err := resolveReviewBase(context.Background(), ".", "")
	require.NoError(t, err)
	assert.Equal(t, "trunk", base)
}

func TestReviewBaseFallsBackToMaster(t *testing.T) {
	stubReviewGit(t, func(_ context.Context, _ string, args ...string) (string, error) {
		switch strings.Join(args, " ") {
		case "rev-parse --verify --quiet refs/heads/master":
			return "", nil
		default:
			return "", errors.New("no such ref")
		}
	})

	base, err := resolveReviewBase(context.Background(), ".", "")
	require.NoError(t, err)
	assert.Equal(t, "master", base)
}

func TestReviewBaseUnresolvableFails(t *testing.T) {
	stubReviewGit(t, func(_ context.Context, _ string, _ ...string) (string, error) {
		return "", errors.New("no such ref")
	})

	_, err := resolveReviewBase(context.Background(), ".", "")
	require.Error(t, err)
	assert.ErrorContains(t, err, "pass --base")
}

func TestReviewBaseFlagMustExist(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	_, err := runReviewCmd(t, repoDir, "", "CB-1", "--base", "release")
	require.Error(t, err)
	assert.ErrorContains(t, err, `base branch "release" does not exist`)
}

// --- merge guards ---

func TestReviewMergeRefusesDirtyTree(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")
	writeReviewFile(t, repoDir, "scratch.txt", "uncommitted\n")

	_, err := runReviewCmd(t, repoDir, "", "CB-1", "--merge")
	require.Error(t, err)
	assert.ErrorContains(t, err, "working tree is dirty")
	assert.True(t, reviewBranchExistsForReal(t, repoDir, "symphony/cb-1"))
}

func TestReviewMergeRefusesWhenNotOnBase(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")
	gitReview(t, repoDir, "checkout", "-b", "other")

	_, err := runReviewCmd(t, repoDir, "", "CB-1", "--merge", "--base", "main")
	require.Error(t, err)
	assert.ErrorContains(t, err, `current branch is "other"`)
}

// --- landing actions against real repos ---

func TestReviewMergeIntegration(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	stdout, err := runReviewCmd(t, repoDir, "", "CB-1", "--merge")
	require.NoError(t, err)
	assert.Contains(t, stdout, "merged symphony/cb-1 into main")

	subject := gitReview(t, repoDir, "log", "-1", "--pretty=%s", "main")
	assert.Contains(t, subject, "merge contrabass run symphony/cb-1")
	assert.FileExists(t, filepath.Join(repoDir, "feature.txt"))
	// A --no-ff merge commit has a second parent.
	gitReview(t, repoDir, "rev-parse", "main^2")
}

func TestReviewSquashIntegration(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	stdout, err := runReviewCmd(t, repoDir, "", "CB-1", "--squash")
	require.NoError(t, err)
	assert.Contains(t, stdout, "squash-merged symphony/cb-1 into main")

	subject := gitReview(t, repoDir, "log", "-1", "--pretty=%s", "main")
	assert.Contains(t, subject, "contrabass run symphony/cb-1 (squashed)")
	assert.FileExists(t, filepath.Join(repoDir, "feature.txt"))

	// A squash commit must have a single parent.
	cmd := exec.Command("git", "rev-parse", "main^2")
	cmd.Dir = repoDir
	assert.Error(t, cmd.Run(), "squash commit must not have a second parent")
}

func TestReviewDiscardIntegration(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	stdout, err := runReviewCmd(t, repoDir, "", "CB-1", "--discard")
	require.NoError(t, err)
	assert.Contains(t, stdout, "discarded branch symphony/cb-1")
	assert.False(t, reviewBranchExistsForReal(t, repoDir, "symphony/cb-1"))
}

// --- feedback hook ---

func TestReviewFeedbackFlagInvokesHook(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	var gotTarget, gotFeedback string
	stubReviewFeedback(t, func(issueOrBranch, feedback string) error {
		gotTarget = issueOrBranch
		gotFeedback = feedback
		return nil
	})

	stdout, err := runReviewCmd(t, repoDir, "", "CB-1", "--feedback", "tighten the tests")
	require.NoError(t, err)
	assert.Equal(t, "CB-1", gotTarget)
	assert.Equal(t, "tighten the tests", gotFeedback)
	assert.Contains(t, stdout, "feedback sent for CB-1")
}

func TestReviewFeedbackDefaultHookRunsResume(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")
	t.Chdir(repoDir)

	// The default hook is wired to the resume command; without a matching
	// board issue it surfaces resume's lookup error.
	_, err := runReviewCmd(t, repoDir, "", "CB-1", "--feedback", "anything")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")
	assert.ErrorContains(t, err, "--board-dir")
}

// --- diff flag ---

func TestReviewDiffFlagPrintsFullDiff(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	stdout, err := runReviewCmd(t, repoDir, "", "CB-1", "--diff")
	require.NoError(t, err)
	assert.Contains(t, stdout, "feature.txt")
	assert.Contains(t, stdout, "+agent change")
	assert.NotContains(t, stdout, "[m]erge", "--diff must skip the prompt")
}

// --- interactive prompt ---

func TestReviewInteractiveMerge(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	stdout, err := runReviewCmd(t, repoDir, "m\n", "CB-1")
	require.NoError(t, err)
	assert.Contains(t, stdout, "[m]erge / [s]quash-merge / [d]iscard / [f]eedback / [q]uit:")
	assert.Contains(t, stdout, "merged symphony/cb-1 into main")

	subject := gitReview(t, repoDir, "log", "-1", "--pretty=%s", "main")
	assert.Contains(t, subject, "merge contrabass run symphony/cb-1")
}

func TestReviewInteractiveDiscardConfirmed(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	stdout, err := runReviewCmd(t, repoDir, "d\ny\n", "CB-1")
	require.NoError(t, err)
	assert.Contains(t, stdout, "delete branch symphony/cb-1? [y/N]:")
	assert.Contains(t, stdout, "discarded branch symphony/cb-1")
	assert.False(t, reviewBranchExistsForReal(t, repoDir, "symphony/cb-1"))
}

func TestReviewInteractiveDiscardCancelled(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	stdout, err := runReviewCmd(t, repoDir, "d\nn\nq\n", "CB-1")
	require.NoError(t, err)
	assert.Contains(t, stdout, "discard cancelled")
	assert.True(t, reviewBranchExistsForReal(t, repoDir, "symphony/cb-1"))
}

func TestReviewInteractiveFeedback(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	var gotTarget, gotFeedback string
	stubReviewFeedback(t, func(issueOrBranch, feedback string) error {
		gotTarget = issueOrBranch
		gotFeedback = feedback
		return nil
	})

	stdout, err := runReviewCmd(t, repoDir, "f\nplease add edge-case tests\n", "CB-1")
	require.NoError(t, err)
	assert.Equal(t, "CB-1", gotTarget)
	assert.Equal(t, "please add edge-case tests", gotFeedback)
	assert.Contains(t, stdout, "feedback sent for CB-1")
}

func TestReviewInteractiveQuitLeavesBranch(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	_, err := runReviewCmd(t, repoDir, "q\n", "CB-1")
	require.NoError(t, err)
	assert.True(t, reviewBranchExistsForReal(t, repoDir, "symphony/cb-1"))
}

func TestReviewNonInteractiveEOFQuits(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	_, err := runReviewCmd(t, repoDir, "", "CB-1")
	require.NoError(t, err)
	assert.True(t, reviewBranchExistsForReal(t, repoDir, "symphony/cb-1"))
}

// --- flag exclusivity ---

func TestReviewActionFlagsAreMutuallyExclusive(t *testing.T) {
	repoDir := initReviewRepo(t)
	addReviewRunBranch(t, repoDir, "symphony/cb-1")

	_, err := runReviewCmd(t, repoDir, "", "CB-1", "--merge", "--squash")
	require.Error(t, err)
	assert.ErrorContains(t, err, "none of the others can be")
}
