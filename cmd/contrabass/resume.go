package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/junhoyeo/contrabass/internal/config"
	"github.com/junhoyeo/contrabass/internal/history"
	"github.com/junhoyeo/contrabass/internal/logging"
	"github.com/junhoyeo/contrabass/internal/tracker"
	"github.com/junhoyeo/contrabass/internal/types"
)

func init() {
	// Wire the review command's feedback action to a real resume run.
	reviewFeedback = resumeFromReview
}

func resumeFromReview(issueOrBranch, feedback string) error {
	issueID := issueOrBranch
	if idx := strings.LastIndex(issueID, "/"); idx >= 0 {
		issueID = issueID[idx+1:]
	}
	cmd := newResumeCmd()
	cmd.SetArgs([]string{strings.ToUpper(issueID), feedback})
	return cmd.Execute()
}

func newResumeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resume <issue-id> [feedback...]",
		Short: "Continue a previous run with feedback",
		Long: `Re-run an issue, reusing its branch and — for the claude agent — the
recorded session so the agent continues with full context instead of
starting over. Trailing arguments become reviewer feedback.`,
		Args: cobra.MinimumNArgs(1),
		RunE: runResume,
	}
	cmd.Flags().String("config", "", "workflow file for agent/model defaults (default: WORKFLOW.md when present)")
	cmd.Flags().String("board-dir", "", "board directory holding the issue (default: one-shot board, then the configured board)")
	cmd.Flags().String("log-file", "contrabass.log", "log output path")
	return cmd
}

func runResume(cmd *cobra.Command, args []string) error {
	issueID := strings.TrimSpace(args[0])
	feedback := strings.TrimSpace(strings.Join(args[1:], " "))

	configPath, _ := cmd.Flags().GetString("config")
	boardDirFlag, _ := cmd.Flags().GetString("board-dir")
	logFile, _ := cmd.Flags().GetString("log-file")

	base, err := loadBaseConfig(configPath, "")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx := cmd.Context()
	boardDir, issue, err := findBoardIssue(ctx, issueID, boardDirFlag, base)
	if err != nil {
		return err
	}
	issueID = issue.ID

	record, _ := latestHistoryRecord(base, issueID)
	cfg := prepareResumeConfig(base, boardDir, record, feedback)

	out := cmd.OutOrStdout()
	if record != nil && record.SessionID != "" && cfg.AgentType() == "claude" {
		fmt.Fprintf(out, "resuming %s from session %s\n", issueID, record.SessionID)
	} else {
		fmt.Fprintf(out, "re-running %s with feedback (no resumable session)\n", issueID)
	}

	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    boardDir,
		IssuePrefix: cfg.LocalIssuePrefix(),
		Actor:       "resume",
	})
	// Park the issue back in a dispatchable state; finished issues are
	// otherwise invisible to the orchestrator.
	if err := localTracker.UpdateIssueState(ctx, issueID, types.Unclaimed); err != nil {
		return fmt.Errorf("reopening issue %s: %w", issueID, err)
	}

	session := newSessionID()
	logger := logging.NewLogger(logging.LogOptions{
		Level:   log.InfoLevel,
		Output:  logFile,
		Prefix:  "contrabass",
		Session: session,
	})

	runner, err := createRunner(cfg, "resume", nil)
	if err != nil {
		return fmt.Errorf("creating agent runner: %w", err)
	}
	defer runner.Close()

	result, err := runSingleIssue(ctx, out, cfg, localTracker, issueID, runner, logger)
	if err != nil {
		return err
	}

	printSingleRunSummary(out, result, issueID)
	if !result.Succeeded {
		return fmt.Errorf("resume failed (phase %s)", result.Phase)
	}
	return nil
}

// findBoardIssue locates the issue on the explicit board, the one-shot
// board, or the configured board — in that order — trying upper-case ID
// variants so branch-derived ids ("cb-12") resolve.
func findBoardIssue(ctx context.Context, issueID, boardDirFlag string, cfg *config.WorkflowConfig) (string, tracker.LocalBoardIssue, error) {
	var dirs []string
	if boardDirFlag != "" {
		dirs = []string{boardDirFlag}
	} else {
		dirs = []string{defaultOneShotBoardDir, cfg.LocalBoardDir()}
	}

	candidates := []string{issueID, strings.ToUpper(issueID)}
	for _, dir := range dirs {
		localTracker := tracker.NewLocalTracker(tracker.LocalConfig{BoardDir: dir})
		for _, candidate := range candidates {
			if issue, err := localTracker.GetIssue(ctx, candidate); err == nil {
				return dir, issue, nil
			}
		}
	}
	return "", tracker.LocalBoardIssue{}, fmt.Errorf(
		"issue %s not found (looked in %s); pass --board-dir", issueID, strings.Join(dirs, ", "))
}

// latestHistoryRecord returns the most recent run record for the issue.
func latestHistoryRecord(cfg *config.WorkflowConfig, issueID string) (*history.Record, error) {
	if !cfg.HistoryEnabled() {
		return nil, nil
	}
	store := history.NewStore(cfg.HistoryDir())
	records, err := store.Records(0)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if strings.EqualFold(records[i].IssueID, issueID) || strings.EqualFold(records[i].Identifier, issueID) {
			return &records[i], nil
		}
	}
	return nil, nil
}

// prepareResumeConfig rebases the workflow onto the issue's board and, when
// the prior run was a claude session, injects --resume so the agent
// continues with context. Feedback is wrapped in liquid raw tags so reviewer
// text can never break template rendering.
func prepareResumeConfig(base *config.WorkflowConfig, boardDir string, record *history.Record, feedback string) *config.WorkflowConfig {
	cfg := prepareSingleRunConfig(base, boardDir)

	if record != nil && record.AgentType != "" {
		cfg.Agent.Type = record.AgentType
	}

	resumable := record != nil && record.SessionID != "" && cfg.AgentType() == "claude"
	if resumable {
		cfg.Claude.ExtraArgs = append(slices.Clone(cfg.Claude.ExtraArgs), "--resume", record.SessionID)
	}

	if feedback != "" {
		wrapped := "{% raw %}" + feedback + "{% endraw %}"
		if resumable {
			// The session already holds the task context; the feedback is
			// the whole prompt.
			cfg.PromptTemplate = "Continue your previous work on this task. Reviewer feedback:\n\n" + wrapped
		} else {
			cfg.PromptTemplate = cfg.PromptTemplate + "\n\n## Reviewer feedback on the previous attempt\n\n" + wrapped
		}
	} else if resumable {
		cfg.PromptTemplate = "Continue your previous work on this task and finish it."
	}

	return cfg
}
