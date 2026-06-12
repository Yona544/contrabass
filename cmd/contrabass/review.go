package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/junhoyeo/contrabass/internal/config"
)

// reviewDefaultBranchPrefix mirrors the workspace.branch_prefix default the
// trackers use when deriving run branches from issue ids.
const reviewDefaultBranchPrefix = "symphony/"

// reviewRunGit executes git inside the repository under review. Package-level
// so unit tests can stub git interactions (mirrors doctorRunCommand).
var reviewRunGit = func(ctx context.Context, repoPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// reviewFeedback forwards review feedback for an agent run. The resume
// command rewires this at startup; the default surfaces a clear error when
// that wiring is absent.
var reviewFeedback = func(issueOrBranch, feedback string) error {
	return errors.New("feedback requires the resume command (wired separately)")
}

func newReviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review <issue-id-or-branch>",
		Short: "Review and land an agent run branch",
		Long: `Review the changes an agent run produced on its branch and decide what to do
with them: merge, squash-merge, discard, or send feedback. The argument is
either an existing local branch or an issue id, which resolves to
<branch_prefix><lowercased-issue-id> (prefix from workspace.branch_prefix when
--config is given, "symphony/" otherwise).`,
		Args: cobra.ExactArgs(1),
		RunE: runReview,
	}
	cmd.Flags().String("config", "", "path to WORKFLOW.md (provides workspace.branch_prefix)")
	cmd.Flags().String("repo", ".", "path to the repository root")
	cmd.Flags().String("base", "", "base branch to compare and merge into (default: the repo's default branch)")
	cmd.Flags().Bool("merge", false, "merge the run branch into the base branch without prompting")
	cmd.Flags().Bool("squash", false, "squash-merge the run branch into the base branch without prompting")
	cmd.Flags().Bool("discard", false, "delete the run branch without prompting")
	cmd.Flags().String("feedback", "", "send feedback for the run instead of landing it")
	cmd.Flags().Bool("diff", false, "print the full diff against the base branch and exit")
	cmd.MarkFlagsMutuallyExclusive("merge", "squash", "discard", "feedback", "diff")
	return cmd
}

func runReview(cmd *cobra.Command, args []string) error {
	target := strings.TrimSpace(args[0])
	if target == "" {
		return errors.New("issue id or branch name is required")
	}

	cfgPath, _ := cmd.Flags().GetString("config")
	repoPath, _ := cmd.Flags().GetString("repo")
	baseFlag, _ := cmd.Flags().GetString("base")
	mergeFlag, _ := cmd.Flags().GetBool("merge")
	squashFlag, _ := cmd.Flags().GetBool("squash")
	discardFlag, _ := cmd.Flags().GetBool("discard")
	feedbackFlag, _ := cmd.Flags().GetString("feedback")
	diffFlag, _ := cmd.Flags().GetBool("diff")

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	out := cmd.OutOrStdout()

	prefix, err := resolveReviewBranchPrefix(cfgPath)
	if err != nil {
		return err
	}

	branch, err := resolveReviewBranch(ctx, repoPath, target, prefix)
	if err != nil {
		return err
	}

	base, err := resolveReviewBase(ctx, repoPath, baseFlag)
	if err != nil {
		return err
	}

	switch {
	case diffFlag:
		return reviewShowFullDiff(ctx, out, repoPath, base, branch)
	case mergeFlag:
		return reviewMerge(ctx, out, repoPath, base, branch)
	case squashFlag:
		return reviewSquash(ctx, out, repoPath, base, branch)
	case discardFlag:
		return reviewDiscard(ctx, out, repoPath, branch)
	case feedbackFlag != "":
		return reviewSendFeedback(out, target, feedbackFlag)
	}

	if err := reviewShowSummary(ctx, out, repoPath, base, branch); err != nil {
		return err
	}
	return reviewPromptLoop(ctx, cmd, repoPath, base, branch, target)
}

// resolveReviewBranchPrefix returns workspace.branch_prefix from the workflow
// config when one is given, falling back to the trackers' default.
func resolveReviewBranchPrefix(cfgPath string) (string, error) {
	if strings.TrimSpace(cfgPath) == "" {
		return reviewDefaultBranchPrefix, nil
	}
	cfg, err := config.ParseWorkflow(cfgPath)
	if err != nil {
		return "", fmt.Errorf("parsing workflow config: %w", err)
	}
	if prefix := strings.TrimSpace(cfg.Workspace.BranchPrefix); prefix != "" {
		return prefix, nil
	}
	return reviewDefaultBranchPrefix, nil
}

// resolveReviewBranch accepts either an existing local branch name or an
// issue id that derives the run branch as <prefix><lowercased-issue-id>.
func resolveReviewBranch(ctx context.Context, repoPath, target, prefix string) (string, error) {
	if reviewBranchExists(ctx, repoPath, target) {
		return target, nil
	}
	derived := prefix + strings.ToLower(target)
	if derived != target && reviewBranchExists(ctx, repoPath, derived) {
		return derived, nil
	}
	if derived == target {
		return "", fmt.Errorf("no local branch %q exists; pass an existing branch name or an issue id", target)
	}
	return "", fmt.Errorf("no local branch %q or %q exists; pass an existing branch name or an issue id", target, derived)
}

// resolveReviewBase picks the branch to compare and merge into: the --base
// flag when given, the branch origin/HEAD points at, then main, then master.
func resolveReviewBase(ctx context.Context, repoPath, baseFlag string) (string, error) {
	if base := strings.TrimSpace(baseFlag); base != "" {
		if !reviewBranchExists(ctx, repoPath, base) {
			return "", fmt.Errorf("base branch %q does not exist", base)
		}
		return base, nil
	}
	if output, err := reviewRunGit(ctx, repoPath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		name := strings.TrimPrefix(strings.TrimSpace(output), "origin/")
		if name != "" && reviewBranchExists(ctx, repoPath, name) {
			return name, nil
		}
	}
	for _, candidate := range []string{"main", "master"} {
		if reviewBranchExists(ctx, repoPath, candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("could not determine the base branch (tried origin/HEAD, main, master); pass --base")
}

func reviewBranchExists(ctx context.Context, repoPath, branch string) bool {
	_, err := reviewRunGit(ctx, repoPath, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func reviewShowSummary(ctx context.Context, out io.Writer, repoPath, base, branch string) error {
	stat, err := reviewRunGit(ctx, repoPath, "diff", "--stat", base+"..."+branch)
	if err != nil {
		return fmt.Errorf("git diff --stat %s...%s failed: %v; output: %s", base, branch, err, strings.TrimSpace(stat))
	}
	commits, err := reviewRunGit(ctx, repoPath, "log", "--oneline", base+".."+branch)
	if err != nil {
		return fmt.Errorf("git log --oneline %s..%s failed: %v; output: %s", base, branch, err, strings.TrimSpace(commits))
	}

	fmt.Fprintf(out, "reviewing %s against %s\n\n", branch, base)
	if stat = strings.TrimRight(stat, "\r\n"); stat != "" {
		fmt.Fprintln(out, stat)
	} else {
		fmt.Fprintln(out, "(no file changes)")
	}
	fmt.Fprintln(out)
	if commits = strings.TrimRight(commits, "\r\n"); commits != "" {
		fmt.Fprintln(out, commits)
	} else {
		fmt.Fprintln(out, "(no commits ahead of base)")
	}
	fmt.Fprintln(out)
	return nil
}

func reviewPromptLoop(ctx context.Context, cmd *cobra.Command, repoPath, base, branch, target string) error {
	reader := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	for {
		fmt.Fprint(out, "[m]erge / [s]quash-merge / [d]iscard / [f]eedback / [q]uit: ")
		line, err := reader.ReadString('\n')
		choice := strings.ToLower(strings.TrimSpace(line))
		if err != nil && choice == "" {
			// Non-interactive stdin (EOF before any choice): leave the
			// branch untouched.
			fmt.Fprintln(out)
			return nil
		}

		switch choice {
		case "m", "merge":
			return reviewMerge(ctx, out, repoPath, base, branch)
		case "s", "squash", "squash-merge":
			return reviewSquash(ctx, out, repoPath, base, branch)
		case "d", "discard":
			fmt.Fprintf(out, "delete branch %s? [y/N]: ", branch)
			answer, _ := reader.ReadString('\n')
			if strings.ToLower(strings.TrimSpace(answer)) != "y" {
				fmt.Fprintln(out, "discard cancelled")
				continue
			}
			return reviewDiscard(ctx, out, repoPath, branch)
		case "f", "feedback":
			fmt.Fprint(out, "feedback: ")
			text, _ := reader.ReadString('\n')
			if text = strings.TrimSpace(text); text == "" {
				fmt.Fprintln(out, "feedback text is empty")
				continue
			}
			return reviewSendFeedback(out, target, text)
		case "q", "quit", "":
			return nil
		default:
			fmt.Fprintf(out, "unknown choice %q\n", choice)
		}
	}
}

// reviewEnsureCleanOnBase refuses to land a branch unless the working tree is
// clean and the base branch is checked out, so the merge result is exactly
// what the reviewer inspected.
func reviewEnsureCleanOnBase(ctx context.Context, repoPath, base string) error {
	status, err := reviewRunGit(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("git status --porcelain failed: %v; output: %s", err, strings.TrimSpace(status))
	}
	if strings.TrimSpace(status) != "" {
		return errors.New("working tree is dirty (uncommitted or untracked changes); commit, stash, or clean it before landing a run branch")
	}

	head, err := reviewRunGit(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("git rev-parse --abbrev-ref HEAD failed: %v; output: %s", err, strings.TrimSpace(head))
	}
	if current := strings.TrimSpace(head); current != base {
		return fmt.Errorf("current branch is %q, not the base branch %q; run `git checkout %s` first", current, base, base)
	}
	return nil
}

func reviewMerge(ctx context.Context, out io.Writer, repoPath, base, branch string) error {
	if err := reviewEnsureCleanOnBase(ctx, repoPath, base); err != nil {
		return err
	}
	output, err := reviewRunGit(ctx, repoPath, "merge", "--no-ff", branch, "-m", "merge contrabass run "+branch)
	if err != nil {
		return fmt.Errorf("git merge --no-ff %s failed: %v; output: %s", branch, err, strings.TrimSpace(output))
	}
	fmt.Fprintf(out, "merged %s into %s\n", branch, base)
	return nil
}

func reviewSquash(ctx context.Context, out io.Writer, repoPath, base, branch string) error {
	if err := reviewEnsureCleanOnBase(ctx, repoPath, base); err != nil {
		return err
	}
	output, err := reviewRunGit(ctx, repoPath, "merge", "--squash", branch)
	if err != nil {
		return fmt.Errorf("git merge --squash %s failed: %v; output: %s", branch, err, strings.TrimSpace(output))
	}
	output, err = reviewRunGit(ctx, repoPath, "commit", "-m", fmt.Sprintf("contrabass run %s (squashed)", branch))
	if err != nil {
		return fmt.Errorf("git commit after squash of %s failed: %v; output: %s", branch, err, strings.TrimSpace(output))
	}
	fmt.Fprintf(out, "squash-merged %s into %s\n", branch, base)
	return nil
}

func reviewDiscard(ctx context.Context, out io.Writer, repoPath, branch string) error {
	output, err := reviewRunGit(ctx, repoPath, "branch", "-D", branch)
	if err != nil {
		return fmt.Errorf("git branch -D %s failed: %v; output: %s", branch, err, strings.TrimSpace(output))
	}
	fmt.Fprintf(out, "discarded branch %s\n", branch)
	return nil
}

func reviewSendFeedback(out io.Writer, issueOrBranch, feedback string) error {
	if err := reviewFeedback(issueOrBranch, feedback); err != nil {
		return fmt.Errorf("sending feedback for %s: %w", issueOrBranch, err)
	}
	fmt.Fprintf(out, "feedback sent for %s\n", issueOrBranch)
	return nil
}

func reviewShowFullDiff(ctx context.Context, out io.Writer, repoPath, base, branch string) error {
	diff, err := reviewRunGit(ctx, repoPath, "diff", base+"..."+branch)
	if err != nil {
		return fmt.Errorf("git diff %s...%s failed: %v; output: %s", base, branch, err, strings.TrimSpace(diff))
	}
	fmt.Fprint(out, diff)
	if diff != "" && !strings.HasSuffix(diff, "\n") {
		fmt.Fprintln(out)
	}
	return nil
}
