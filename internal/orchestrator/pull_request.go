package orchestrator

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/junhoyeo/contrabass/internal/logging"
	"github.com/junhoyeo/contrabass/internal/timeline"
	"github.com/junhoyeo/contrabass/internal/types"
)

const prCommandTimeout = 60 * time.Second

// PullRequestConfig turns verified successful runs into pull requests: the
// workspace branch is pushed and a PR is opened via the gh CLI. Failures are
// best-effort — the run already succeeded, so PR problems are logged and
// commented but never fail the run.
type PullRequestConfig struct {
	Enabled bool
	// Draft opens the PR as a draft (recommended: a human still reviews).
	Draft bool
	// Base overrides the repository's default base branch when set.
	Base string
	// Remote is the git remote to push to; defaults to origin.
	Remote string
}

// prExecutor runs a git/gh command inside dir and returns combined output.
// Injectable so tests never shell out.
type prExecutor func(ctx context.Context, dir, name string, args ...string) (string, error)

func defaultPRExecutor(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, prCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, text)
		}
		return text, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return text, nil
}

// SetPullRequestConfig enables the post-verify auto-PR step.
func (o *Orchestrator) SetPullRequestConfig(cfg PullRequestConfig) {
	if cfg.Remote == "" {
		cfg.Remote = "origin"
	}
	o.prConfig = cfg
}

// maybeOpenPullRequest pushes the workspace branch and opens (or reuses) a PR
// for a verified successful run. It must run before workspace cleanup while
// the worktree still exists.
func (o *Orchestrator) maybeOpenPullRequest(ctx context.Context, entry *runEntry, attempt types.RunAttempt) {
	if !o.prConfig.Enabled || entry == nil {
		return
	}

	execCmd := o.prExec
	if execCmd == nil {
		execCmd = defaultPRExecutor
	}

	workspace := entry.workspace
	issue := entry.issue

	branch := strings.TrimSpace(issue.BranchName)
	if branch == "" {
		resolved, err := execCmd(ctx, workspace, "git", "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil || resolved == "" || resolved == "HEAD" {
			o.reportPullRequestFailure(ctx, issue, attempt, fmt.Errorf("resolve branch: %v", err))
			return
		}
		branch = resolved
	}

	if _, err := execCmd(ctx, workspace, "git", "push", "-u", o.prConfig.Remote, branch); err != nil {
		o.reportPullRequestFailure(ctx, issue, attempt, fmt.Errorf("push branch %s: %w", branch, err))
		return
	}

	// Reuse an existing PR for the branch (retried runs must stay idempotent).
	if url, err := execCmd(ctx, workspace, "gh", "pr", "view", branch, "--json", "url", "--jq", ".url"); err == nil && strings.HasPrefix(url, "http") {
		o.reportPullRequestOpened(ctx, issue, attempt, url, true)
		return
	}

	title := strings.TrimSpace(fmt.Sprintf("%s: %s", issue.Identifier, issue.Title))
	title = strings.TrimPrefix(title, ": ")
	body := fmt.Sprintf(
		"Automated run for %s (attempt %d).\n\nIssue: %s\nTokens: %d in / %d out\n\n🤖 Opened by [contrabass](https://github.com/junhoyeo/contrabass)",
		issue.Identifier, attempt.Attempt, issue.URL, attempt.TokensIn, attempt.TokensOut,
	)

	args := []string{"pr", "create", "--head", branch, "--title", title, "--body", body}
	if o.prConfig.Draft {
		args = append(args, "--draft")
	}
	if o.prConfig.Base != "" {
		args = append(args, "--base", o.prConfig.Base)
	}

	output, err := execCmd(ctx, workspace, "gh", args...)
	if err != nil {
		o.reportPullRequestFailure(ctx, issue, attempt, fmt.Errorf("create pull request: %w", err))
		return
	}

	url := lastNonEmptyLine(output)
	o.reportPullRequestOpened(ctx, issue, attempt, url, false)
}

func (o *Orchestrator) reportPullRequestOpened(ctx context.Context, issue types.Issue, attempt types.RunAttempt, url string, reused bool) {
	action := "opened"
	if reused {
		action = "updated"
	}
	logging.LogIssueEvent(o.logger, issue.ID, "pull_request_"+action, "url", url)
	o.recordTimelineNode(ctx, issue, attempt,
		"pull-request", timeline.NodeStatusSucceeded, "Pull request "+action,
		"The verified branch was pushed and a pull request is ready for review.", "", true)
	if !o.shouldSuppressLegacyComment() {
		if err := o.tracker.PostComment(ctx, issue.ID, fmt.Sprintf("Pull request %s: %s", action, url)); err != nil {
			logging.LogIssueEvent(o.logger, issue.ID, "post_comment_failed", "err", err)
		}
	}
}

func (o *Orchestrator) reportPullRequestFailure(ctx context.Context, issue types.Issue, attempt types.RunAttempt, prErr error) {
	logging.LogIssueEvent(o.logger, issue.ID, "pull_request_failed", "err", prErr)
	o.recordTimelineNode(ctx, issue, attempt,
		"pull-request", timeline.NodeStatusFailed, "Pull request failed",
		"The run succeeded but the branch could not be turned into a pull request.", prErr.Error(), true)
	if !o.shouldSuppressLegacyComment() {
		if err := o.tracker.PostComment(ctx, issue.ID, fmt.Sprintf("Pull request creation failed: %v", prErr)); err != nil {
			logging.LogIssueEvent(o.logger, issue.ID, "post_comment_failed", "err", err)
		}
	}
}

func lastNonEmptyLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}
