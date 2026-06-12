package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/junhoyeo/contrabass/internal/agent"
	"github.com/junhoyeo/contrabass/internal/config"
	"github.com/junhoyeo/contrabass/internal/history"
	"github.com/junhoyeo/contrabass/internal/logging"
	"github.com/junhoyeo/contrabass/internal/orchestrator"
	"github.com/junhoyeo/contrabass/internal/tracker"
	"github.com/junhoyeo/contrabass/internal/types"
	"github.com/junhoyeo/contrabass/internal/workspace"
)

const (
	defaultOneShotBoardDir = ".contrabass/oneshot/board"
	oneShotIssuePrefix     = "RUN"
	// singleRunGrace bounds how long we wait for the orchestrator's
	// completion path (PR, cleanup, release) after the agent finishes.
	singleRunGrace = 60 * time.Second
)

// staticConfigProvider satisfies orchestrator.ConfigProvider for commands
// that run with a fixed, already-resolved config (no live reload).
type staticConfigProvider struct {
	cfg *config.WorkflowConfig
}

func (p staticConfigProvider) GetConfig() *config.WorkflowConfig { return p.cfg }

// singleIssueTracker narrows a tracker to one issue so the orchestrator
// cannot pick up unrelated backlog entries during a one-shot run.
type singleIssueTracker struct {
	tracker.Tracker
	issueID string
}

func (t singleIssueTracker) FetchIssues(ctx context.Context) ([]types.Issue, error) {
	issues, err := t.Tracker.FetchIssues(ctx)
	if err != nil {
		return nil, err
	}
	filtered := issues[:0]
	for _, issue := range issues {
		if issue.ID == t.issueID || issue.Identifier == t.issueID {
			filtered = append(filtered, issue)
		}
	}
	return filtered, nil
}

type singleRunResult struct {
	Succeeded bool
	Phase     string
	Error     string
	TokensIn  int64
	TokensOut int64
	CostUSD   float64
	Branch    string
	SessionID string
	Workspace string
}

// runSingleIssue drives exactly one issue through the orchestrator and
// returns once its completion path (release or backoff) finishes. The caller
// owns the runner so tests can inject mocks.
func runSingleIssue(
	ctx context.Context,
	out io.Writer,
	cfg *config.WorkflowConfig,
	localTracker *tracker.LocalTracker,
	issueID string,
	runner agent.AgentRunner,
	logger *log.Logger,
) (*singleRunResult, error) {
	repoPath, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	workspaceMgr := workspace.NewManager(repoPath)
	workspaceMgr.SetBeforeRemoveHook(cfg.HookBeforeRemove())

	orch := orchestrator.NewOrchestrator(
		singleIssueTracker{Tracker: localTracker, issueID: issueID},
		workspaceMgr, runner, staticConfigProvider{cfg: cfg}, logger,
	)
	orch.SetPullRequestConfig(orchestrator.PullRequestConfig{
		Enabled: cfg.PullRequest.Enabled,
		Draft:   cfg.PullRequestDraft(),
		Base:    cfg.PullRequest.Base,
		Remote:  cfg.PullRequest.Remote,
	})

	var historyStore *history.Store
	if cfg.HistoryEnabled() {
		historyStore = history.NewStore(cfg.HistoryDir())
		historyStore.SetPricing(modelPricing(cfg))
		orch.SetRunHistory(historyStore)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	result := &singleRunResult{}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		var graceTimer *time.Timer
		var graceC <-chan time.Time
		events := orch.Events()
		for {
			select {
			case <-graceC:
				cancel()
				graceC = nil
			case event, ok := <-events:
				if !ok {
					return
				}
				if event.IssueID != "" && event.IssueID != issueID {
					continue
				}
				switch data := event.Data.(type) {
				case orchestrator.AgentStarted:
					result.SessionID = data.SessionID
					result.Workspace = data.Workspace
					fmt.Fprintf(out, "agent started (attempt %d, pid %d)\n", data.Attempt, data.PID)
				case orchestrator.AgentFinished:
					result.Phase = data.Phase.String()
					result.Error = data.Error
					result.TokensIn = data.TokensIn
					result.TokensOut = data.TokensOut
					result.Succeeded = data.Phase == types.Succeeded
					fmt.Fprintf(out, "agent finished (phase %s, tokens %d in / %d out)\n",
						data.Phase, data.TokensIn, data.TokensOut)
					// Completion bookkeeping (PR, cleanup, release) follows;
					// the terminal events below cancel sooner when they fire.
					if graceTimer == nil {
						graceTimer = time.NewTimer(singleRunGrace)
						graceC = graceTimer.C
					}
				case orchestrator.IssueReleased:
					cancel()
				case orchestrator.BackoffEnqueued:
					if result.Error == "" {
						result.Error = data.Error
					}
					cancel()
				}
			}
		}
	}()

	runErr := orch.Run(runCtx)
	<-finished
	if runErr != nil && ctx.Err() == nil && runCtx.Err() == nil {
		return nil, fmt.Errorf("orchestrator run: %w", runErr)
	}

	if issue, getErr := localTracker.GetIssue(ctx, issueID); getErr == nil {
		result.Branch = issue.BranchName
	}
	if historyStore != nil {
		result.CostUSD = historyStore.Cost(effectiveModelForConfig(cfg), result.TokensIn, result.TokensOut)
	}
	return result, nil
}

// effectiveModelForConfig mirrors the orchestrator's model resolution for
// cost estimates printed by run/resume.
func effectiveModelForConfig(cfg *config.WorkflowConfig) string {
	switch cfg.AgentType() {
	case "claude":
		return cfg.ClaudeModel()
	case "codex":
		return cfg.CodexModel()
	default:
		model, _ := cfg.Model()
		return model
	}
}

// loadBaseConfig parses the given workflow file, falls back to WORKFLOW.md
// in the working directory, and synthesizes a minimal config when neither
// exists.
func loadBaseConfig(configPath, agentType string) (*config.WorkflowConfig, error) {
	if configPath != "" {
		return config.ParseWorkflow(configPath)
	}
	if _, err := os.Stat("WORKFLOW.md"); err == nil {
		return config.ParseWorkflow("WORKFLOW.md")
	}

	if agentType == "" {
		agentType = "claude"
	}
	cfg := &config.WorkflowConfig{}
	cfg.Agent.Type = agentType
	cfg.ModelRaw = defaultModelForAgent(agentType)
	cfg.PromptTemplate = "{{ issue.description }}"
	return cfg, nil
}

// prepareSingleRunConfig rebases any workflow config onto an isolated
// internal board with single-agent execution and no schedule gating —
// explicit one-shots run now, not in the next window.
func prepareSingleRunConfig(base *config.WorkflowConfig, boardDir string) *config.WorkflowConfig {
	cfg := base.Clone()
	cfg.Tracker = config.TrackerConfig{Type: "internal", BoardDir: boardDir, IssuePrefix: oneShotIssuePrefix}
	cfg.Team.ExecutionMode = config.TeamExecutionModeSingle
	cfg.MaxConcurrencyRaw = 1
	cfg.PollIntervalMsRaw = 1000
	cfg.Schedule = config.ScheduleConfig{}
	if strings.TrimSpace(cfg.PromptTemplate) == "" {
		cfg.PromptTemplate = "{{ issue.description }}"
	}
	return cfg
}

func oneShotTitle(prompt, explicit string) string {
	if explicit != "" {
		return explicit
	}
	title := strings.TrimSpace(strings.SplitN(prompt, "\n", 2)[0])
	if len(title) > 72 {
		title = title[:69] + "..."
	}
	return title
}

func printSingleRunSummary(out io.Writer, result *singleRunResult, issueID string) {
	status := "FAILED"
	if result.Succeeded {
		status = "SUCCEEDED"
	}
	fmt.Fprintf(out, "\n%s %s (phase %s)\n", status, issueID, result.Phase)
	if result.Error != "" {
		fmt.Fprintf(out, "  error:  %s\n", result.Error)
	}
	fmt.Fprintf(out, "  tokens: %d in / %d out", result.TokensIn, result.TokensOut)
	if result.CostUSD > 0 {
		fmt.Fprintf(out, " (~$%.4f)", result.CostUSD)
	}
	fmt.Fprintln(out)
	if result.Branch != "" {
		fmt.Fprintf(out, "  branch: %s\n", result.Branch)
		fmt.Fprintf(out, "\nNext: contrabass review %s\n", issueID)
	}
}

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <prompt...>",
		Short: "Run a one-shot agent task from a prompt",
		Long: `Run a single agent task without writing a workflow file or creating board
issues by hand. The prompt becomes an ephemeral internal-board issue, the
configured agent runs once in an isolated worktree, and the result is ready
for "contrabass review".`,
		Args: cobra.MinimumNArgs(1),
		RunE: runOneShot,
	}
	cmd.Flags().String("config", "", "workflow file for agent/model defaults (default: WORKFLOW.md when present)")
	cmd.Flags().String("agent", "", "agent type override (claude, codex, opencode, ...)")
	cmd.Flags().String("model", "", "model override")
	cmd.Flags().String("title", "", "issue title (default: first prompt line)")
	cmd.Flags().String("board-dir", defaultOneShotBoardDir, "board directory for one-shot issues")
	cmd.Flags().String("log-file", "contrabass.log", "log output path")
	return cmd
}

func runOneShot(cmd *cobra.Command, args []string) error {
	prompt := strings.TrimSpace(strings.Join(args, " "))
	if prompt == "" {
		return fmt.Errorf("prompt is required")
	}

	configPath, _ := cmd.Flags().GetString("config")
	agentType, _ := cmd.Flags().GetString("agent")
	model, _ := cmd.Flags().GetString("model")
	title, _ := cmd.Flags().GetString("title")
	boardDir, _ := cmd.Flags().GetString("board-dir")
	logFile, _ := cmd.Flags().GetString("log-file")

	base, err := loadBaseConfig(configPath, agentType)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	cfg := prepareSingleRunConfig(base, boardDir)
	if agentType != "" {
		cfg.Agent.Type = agentType
	}
	if model != "" {
		cfg.ModelRaw = model
		cfg.Claude.Model = ""
		cfg.Codex.Model = ""
	}
	if !knownAgentTypes[cfg.AgentType()] {
		return fmt.Errorf("unknown agent type %q", cfg.AgentType())
	}

	ctx := cmd.Context()
	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    boardDir,
		IssuePrefix: oneShotIssuePrefix,
		Actor:       "oneshot",
	})
	issue, err := localTracker.CreateIssueWithOptions(ctx, tracker.LocalIssueCreateOptions{
		Title:       oneShotTitle(prompt, title),
		Description: prompt,
		Labels:      []string{"oneshot"},
	})
	if err != nil {
		return fmt.Errorf("creating one-shot issue: %w", err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "created %s: %s\n", issue.ID, issue.Title)

	session := newSessionID()
	logger := logging.NewLogger(logging.LogOptions{
		Level:   log.InfoLevel,
		Output:  logFile,
		Prefix:  "contrabass",
		Session: session,
	})

	runner, err := createRunner(cfg, "oneshot", nil)
	if err != nil {
		return fmt.Errorf("creating agent runner: %w", err)
	}
	defer runner.Close()

	result, err := runSingleIssue(ctx, out, cfg, localTracker, issue.ID, runner, logger)
	if err != nil {
		return err
	}

	printSingleRunSummary(out, result, issue.ID)
	if !result.Succeeded {
		return fmt.Errorf("run failed (phase %s)", result.Phase)
	}
	return nil
}
