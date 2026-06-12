package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	contrabass "github.com/junhoyeo/contrabass"
	"github.com/junhoyeo/contrabass/internal/config"
	"github.com/junhoyeo/contrabass/internal/history"
	"github.com/junhoyeo/contrabass/internal/hub"
	"github.com/junhoyeo/contrabass/internal/logging"
	"github.com/junhoyeo/contrabass/internal/notify"
	"github.com/junhoyeo/contrabass/internal/orchestrator"
	"github.com/junhoyeo/contrabass/internal/timeline"
	"github.com/junhoyeo/contrabass/internal/tracker"
	"github.com/junhoyeo/contrabass/internal/tui"
	"github.com/junhoyeo/contrabass/internal/update"
	"github.com/junhoyeo/contrabass/internal/web"
	"github.com/junhoyeo/contrabass/internal/workspace"
)

// Build-time variables injected via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var (
	runTUIOrchestrator = func(ctx context.Context, orch *orchestrator.Orchestrator) error {
		return orch.Run(ctx)
	}
	runGracefulShutdown = orchestrator.GracefulShutdown
	runTUIProgram       = func(p *tea.Program) (tea.Model, error) {
		return p.Run()
	}
	startTUIEventBridge = func(ctx context.Context, p *tea.Program, events <-chan orchestrator.OrchestratorEvent) {
		tui.StartEventBridge(ctx, p, events)
	}
	runRootTeamExecution = func(
		ctx context.Context,
		cfgPath string,
		watcher *config.Watcher,
		logger *log.Logger,
		noTUI bool,
		dryRun bool,
		webOpts webOptions,
	) error {
		return runTeamExecutionApp(ctx, cfgPath, watcher, logger, noTUI, dryRun, webOpts)
	}
	runTUIShutdownTimeout = 6 * time.Second
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// newRootCmd builds the Cobra root command with all CLI flags.
func newRootCmd() *cobra.Command {
	var (
		cfgPath  string
		noTUI    bool
		logFile  string
		logLevel string
		dryRun   bool
		port     int
		listen   string
	)

	var updateResult update.Result

	cmd := &cobra.Command{
		Use:     "contrabass",
		Short:   "Orchestrate coding agents with a Charm TUI dashboard",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
		Long: `Contrabass is a Go reimplementation of OpenAI's Symphony.
It orchestrates coding agents against an issue tracker and visualises
progress in a terminal UI built with the Charm stack.`,
		SilenceUsage: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			updateResult = update.Check(context.Background(), version)
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if msg := update.FormatNotification(updateResult); msg != "" {
				fmt.Fprint(os.Stderr, msg)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cfgPath, noTUI, logFile, logLevel, dryRun, port, listen)
		},
	}

	cmd.Flags().StringVar(&cfgPath, "config", "", "path to WORKFLOW.md file (required)")
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "headless mode — skip TUI, log events to stdout")
	cmd.Flags().StringVar(&logFile, "log-file", "contrabass.log", "log output path")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "log level (debug/info/warn/error)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "exit after first poll cycle")
	cmd.Flags().IntVar(&port, "port", 0, "web dashboard port (0 = disabled)")
	cmd.Flags().StringVar(&listen, "listen", "", "web dashboard listen address (host:port); non-loopback hosts require web.auth_token or CONTRABASS_DASHBOARD_TOKEN")

	_ = cmd.MarkFlagRequired("config")

	cmd.AddCommand(teamCmd, newBoardCmd(), newDoctorCmd(), newInitCmd(), newValidateCmd())

	return cmd
}

// parseLogLevel converts a string log level to the charmbracelet/log Level.
func parseLogLevel(s string) log.Level {
	switch strings.ToLower(s) {
	case "debug":
		return log.DebugLevel
	case "warn":
		return log.WarnLevel
	case "error":
		return log.ErrorLevel
	default:
		return log.InfoLevel
	}
}

// newSessionID returns an 8-character hex token uniquely identifying this run,
// so concurrent contrabass instances do not interleave entries into a shared log
// file. Falls back to a nanosecond timestamp if crypto/rand is unavailable.
func newSessionID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 16)
}

// run is the main entry point wired into the root command's RunE.
func run(cfgPath string, noTUI bool, logFile, logLevel string, dryRun bool, port int, listen string) error {
	// 1. Parse and validate workflow config
	cfg, err := config.ParseWorkflow(cfgPath)
	if err != nil {
		return fmt.Errorf("parsing workflow config: %w", err)
	}

	webOpts, err := resolveWebOptions(cfg, listen, port)
	if err != nil {
		return err
	}

	// 2. Create logger
	session := newSessionID()
	resolvedLogFile := logging.ResolveLogPath(logFile, session)
	logger := logging.NewLogger(logging.LogOptions{
		Level:   parseLogLevel(logLevel),
		Output:  logFile,
		Prefix:  "contrabass",
		Session: session,
	})
	logTarget := resolvedLogFile
	if logTarget == "" {
		logTarget = "stderr"
	}
	fmt.Fprintf(os.Stderr, "contrabass session=%s log=%s\n", session, logTarget)

	// 3. Create config watcher (live reload via fsnotify)
	watcher, err := config.NewWatcher(cfgPath)
	if err != nil {
		return fmt.Errorf("creating config watcher: %w", err)
	}
	defer watcher.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 5. Start watching config file for changes
	go func() {
		if watchErr := watcher.Watch(ctx); watchErr != nil {
			logger.Error("config watcher failed", "err", watchErr)
		}
	}()

	switch cfg.TeamExecutionMode() {
	case config.TeamExecutionModeTeam:
		return runRootTeamExecution(ctx, cfgPath, watcher, logger, noTUI, dryRun, webOpts)
	case config.TeamExecutionModeSingle:
		// Continue into the original single-agent orchestrator path.
	default:
		return fmt.Errorf(
			"unknown team.execution_mode: %q (supported: auto, team, single)",
			cfg.Team.ExecutionMode,
		)
	}

	// 6. Create tracker
	var trackerClient tracker.Tracker
	switch cfg.TrackerType() {
	case "linear":
		assigneeID := trackerAssigneeID(cfg)
		linearClient, linearErr := tracker.NewLinearClient(tracker.LinearConfig{
			APIKey:      linearAPIKey(cfg),
			ProjectSlug: projectSlug(cfg),
			AssigneeID:  assigneeID,
		})
		if linearErr != nil {
			return fmt.Errorf("creating linear tracker client: %w", linearErr)
		}
		if assigneeID == "" {
			logger.Info("no assignee configured, resolving from API token...")
			viewerID, viewerErr := linearClient.FetchViewerID(ctx)
			if viewerErr != nil {
				logger.Warn("could not auto-resolve assignee from API token", "err", viewerErr)
				logger.Warn("set tracker.assignee_id or LINEAR_ASSIGNEE to claim issues")
			} else {
				linearClient.SetAssigneeID(viewerID)
				logger.Info("auto-resolved assignee from API token", "id", viewerID)
			}
		}
		trackerClient = linearClient
	case "github":
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" {
			token = cfg.GitHubToken()
		}
		owner := os.Getenv("GITHUB_OWNER")
		if owner == "" {
			owner = cfg.GitHubOwner()
		}
		repo := os.Getenv("GITHUB_REPO")
		if repo == "" {
			repo = cfg.GitHubRepo()
		}
		githubClient, githubErr := tracker.NewGitHubClient(tracker.GitHubConfig{
			APIToken: token,
			Owner:    owner,
			Repo:     repo,
			Labels:   cfg.GitHubLabels(),
			Assignee: cfg.GitHubAssignee(),
			Endpoint: cfg.GitHubEndpoint(),
		})
		if githubErr != nil {
			return fmt.Errorf("creating github tracker client: %w", githubErr)
		}
		trackerClient = githubClient
	case "jira":
		jiraClient, jiraErr := tracker.NewJiraClient(jiraConfig(cfg))
		if jiraErr != nil {
			return fmt.Errorf("creating jira tracker client: %w", jiraErr)
		}
		trackerClient = jiraClient
	case "internal", "local":
		trackerClient = tracker.NewLocalTracker(tracker.LocalConfig{
			BoardDir:    cfg.LocalBoardDir(),
			IssuePrefix: cfg.LocalIssuePrefix(),
			Actor:       cfg.GitHubAssignee(),
		})
	default:
		return fmt.Errorf("unknown tracker type: %q (supported: internal, local, linear, github, jira)", cfg.TrackerType())
	}

	// 7. Create workspace manager (uses cwd as repo root)
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	workspaceMgr := workspace.NewManager(repoPath)
	workspaceMgr.SetBeforeRemoveHook(cfg.HookBeforeRemove())

	// 8. Create agent runner (reuses createRunner from team.go)
	agentRunner, err := createRunner(cfg, "orchestrator", nil)
	if err != nil {
		return fmt.Errorf("creating agent runner: %w", err)
	}

	defer agentRunner.Close()

	// 9. Create orchestrator
	orch := orchestrator.NewOrchestrator(trackerClient, workspaceMgr, agentRunner, watcher, logger)
	orch.SetBuildInfo(orchestrator.BuildInfo{Version: version, Commit: commit, Date: date})
	orch.SetPullRequestConfig(orchestrator.PullRequestConfig{
		Enabled: cfg.PullRequest.Enabled,
		Draft:   cfg.PullRequestDraft(),
		Base:    cfg.PullRequest.Base,
		Remote:  cfg.PullRequest.Remote,
	})
	gate, err := buildScheduleGate(cfg)
	if err != nil {
		return err
	}
	if gate != nil {
		orch.SetDispatchGate(gate)
	}
	var historyStore *history.Store
	if cfg.HistoryEnabled() {
		historyStore = history.NewStore(cfg.HistoryDir())
		orch.SetRunHistory(historyStore)
	}
	timelineStore := timeline.NewStore(cfg.WorkflowTimelineDir())
	orch.SetWorkflowTimeline(timelineStore, cfg.LinearSyncCommentsEnabled())
	var linearSyncer *timeline.LinearSyncer
	if cfg.LinearSyncCommentsEnabled() {
		if cfg.TrackerType() == "linear" {
			writer, ok := trackerClient.(tracker.LinearCommentWriter)
			if !ok {
				return fmt.Errorf("linear comment sync enabled but tracker does not support Linear comment writer")
			}
			linearSyncer = timeline.NewLinearSyncer(timelineStore, writer, timeline.LinearSyncerConfig{
				Mode:               cfg.LinearSyncCommentsMode(),
				AllowReplyFallback: !cfg.LinearSyncCommentsModeExplicit(),
				QueueSize:          cfg.LinearSyncCommentsQueueSize(),
				PollInterval:       time.Duration(cfg.LinearSyncCommentsPollIntervalMs()) * time.Millisecond,
				Logger:             logger,
			})
			go func() {
				if err := linearSyncer.Run(ctx); err != nil {
					logger.Warn("linear comment syncer stopped", "err", err)
				}
			}()
		} else {
			logger.Warn("linear comment sync enabled for non-linear tracker; legacy comments remain active",
				"tracker_type", cfg.TrackerType())
		}
	}
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signalChan)
	startSignalShutdownHook(ctx, signalChan, cancel, orch, logger)

	if dryRun {
		return runDryRun(ctx, orch)
	}

	notifier := newNotifier(cfg, logger)

	var h *hub.Hub[web.WebEvent]
	if webOpts.Enabled || notifier.Enabled() {
		webEvents := make(chan web.WebEvent, 256)
		h = hub.NewHub(webEvents)
		go h.Run(ctx)

		go func() {
			for orchEvent := range orch.Events() {
				webEvents <- web.NewOrchestratorWebEvent(orchEvent)
			}
			close(webEvents)
		}()

		if notifier.Enabled() {
			go notifier.Start(ctx)
			go notifier.RunHub(ctx, h)
		}
	}

	if webOpts.Enabled {
		var dashboardFS fs.FS
		if _, statErr := fs.Stat(contrabass.DashboardDistFS, "packages/dashboard/dist"); statErr == nil {
			dashboardFS, err = fs.Sub(contrabass.DashboardDistFS, "packages/dashboard/dist")
			if err != nil {
				return fmt.Errorf("sub dashboard dist fs: %w", err)
			}
		}

		srv := web.NewServer(webOpts.ListenAddr, orch, h, dashboardFS)
		srv.SetAuthToken(webOpts.AuthToken)
		srv.SetAgentStopper(orch)
		srv.SetDispatchController(orch)
		if historyStore != nil {
			srv.SetHistoryProvider(historyStore)
		}
		if detailProvider, ok := trackerClient.(tracker.IssueDetailProvider); ok {
			srv.SetIssueDetailProvider(detailProvider)
		}
		srv.SetTimelineProvider(timelineStore)
		listener, err := net.Listen("tcp", srv.ListenAddr())
		if err != nil {
			return fmt.Errorf("listen web dashboard: %w", err)
		}
		go func() {
			if err := srv.Serve(ctx, listener); err != nil {
				logger.Error("web server error", "err", err)
			}
		}()

		fmt.Fprintf(os.Stderr, "Web dashboard available at %s\n", webOpts.dashboardURL())
	}

	// 10. Select run mode
	if noTUI {
		return runHeadless(ctx, orch, logger, h)
	}
	return runTUI(ctx, orch, h)
}

// runDryRun starts the orchestrator and exits after the first emitted event.
// If no event arrives within the timeout, it logs a warning and returns nil.
func runDryRun(ctx context.Context, orch *orchestrator.Orchestrator) error {
	dryCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	go func() {
		if _, ok := <-orch.Events(); ok {
			cancel()
		}
	}()

	err := orch.Run(dryCtx)
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		log.Warn("dry-run timeout: no events received within 60s")
		return nil
	}
	return err
}

// runHeadless runs the orchestrator without TUI, logging events to the logger.
func runHeadless(
	ctx context.Context,
	orch *orchestrator.Orchestrator,
	logger *log.Logger,
	h *hub.Hub[web.WebEvent],
) error {
	if h != nil {
		subID, subscribedEvents := h.Subscribe()
		defer h.Unsubscribe(subID)
		go func() {
			for webEvt := range subscribedEvents {
				logger.Info("event",
					"kind", string(webEvt.Kind),
					"type", webEvt.Type,
				)
			}
		}()
	} else {
		go func() {
			for event := range orch.Events() {
				logger.Info("event",
					"type", event.Type.String(),
					"issue_id", event.IssueID,
				)
			}
		}()
	}

	return orch.Run(ctx)
}

type workflowTimelineProvider struct {
	store *timeline.Store
}

func (p workflowTimelineProvider) IssueTimeline(ctx context.Context, issueID string) (interface{}, error) {
	if p.store == nil {
		return nil, errors.New("timeline store is nil")
	}
	return p.store.Snapshot(ctx, issueID)
}

func startSignalShutdownHook(
	ctx context.Context,
	signalChan <-chan os.Signal,
	cancel context.CancelFunc,
	orch *orchestrator.Orchestrator,
	logger *log.Logger,
) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-signalChan:
			if shutdownErr := runGracefulShutdown(cancel, orch, orchestrator.DefaultShutdownConfig(), logger); shutdownErr != nil {
				logger.Error("graceful shutdown failed", "err", shutdownErr)
			}
		}
	}()
}

// runTUI starts the orchestrator and renders the Charm TUI.
// When a hub is provided (web dashboard active), the TUI subscribes to the hub
// instead of reading orch.Events() directly — otherwise both consumers would
// compete for the same channel and randomly split events.
func runTUI(
	ctx context.Context,
	orch *orchestrator.Orchestrator,
	h *hub.Hub[web.WebEvent],
) error {
	tuiCtx, tuiCancel := context.WithCancel(ctx)
	defer tuiCancel()

	model := tui.NewModel()
	p := tea.NewProgram(withViewportProgramOptions(model))

	if h != nil {
		subID, webEvents := h.Subscribe()
		defer h.Unsubscribe(subID)
		orchEvents := make(chan orchestrator.OrchestratorEvent, 256)
		go func() {
			defer close(orchEvents)
			for {
				select {
				case <-tuiCtx.Done():
					return
				case we, ok := <-webEvents:
					if !ok {
						return
					}
					if we.Kind == web.WebEventOrchestrator {
						if oe, ok := we.Payload.(orchestrator.OrchestratorEvent); ok {
							orchEvents <- oe
						}
					}
				}
			}
		}()
		startTUIEventBridge(tuiCtx, p, orchEvents)
	} else {
		startTUIEventBridge(tuiCtx, p, orch.Events())
	}

	orchDone := make(chan error, 1)
	orchestratorRunner := runTUIOrchestrator
	go func() {
		defer func() {
			if r := recover(); r != nil {
				orchDone <- fmt.Errorf("orchestrator panic: %v", r)
			}
		}()
		orchDone <- orchestratorRunner(tuiCtx, orch)
	}()

	_, tuiErr := runTUIProgram(p)
	// Clean up native Kitty image AFTER alt-screen exit so the delete
	// command targets the main screen where the image persists.
	tui.CleanupNativeImage()

	// TUI exited — cancel orchestrator context and wait for graceful shutdown
	tuiCancel()
	select {
	case orchErr := <-orchDone:
		if orchErr != nil {
			if tuiErr != nil {
				return fmt.Errorf("orchestrator failed: %w (tui error: %v)", orchErr, tuiErr)
			}
			return orchErr
		}
	case <-time.After(runTUIShutdownTimeout):
		if tuiErr != nil {
			return fmt.Errorf("timed out waiting for orchestrator shutdown: %w", tuiErr)
		}
		return errors.New("timed out waiting for orchestrator shutdown")
	}

	return tuiErr
}

type viewportProgramModel struct {
	model tea.Model
}

func withViewportProgramOptions(model tea.Model) tea.Model {
	return viewportProgramModel{model: model}
}

func (m viewportProgramModel) Init() tea.Cmd {
	return m.model.Init()
}

func (m viewportProgramModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.model.Update(msg)
	m.model = next
	return m, cmd
}

func (m viewportProgramModel) View() tea.View {
	v := m.model.View()
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// projectSlug extracts the Linear project slug from env or config.
func projectSlug(cfg *config.WorkflowConfig) string {
	if envSlug := os.Getenv("LINEAR_PROJECT_SLUG"); envSlug != "" {
		return envSlug
	}
	url, err := cfg.ProjectURL()
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.TrimRight(url, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// linearAPIKey extracts the Linear API key from env with config fallback.
func linearAPIKey(cfg *config.WorkflowConfig) string {
	if envKey := os.Getenv("LINEAR_API_KEY"); envKey != "" {
		return envKey
	}
	if cfg == nil {
		return ""
	}
	return cfg.Tracker.Token
}

// trackerAssigneeID extracts assignee from config with env fallback.
func trackerAssigneeID(cfg *config.WorkflowConfig) string {
	if cfgAssignee := cfg.TrackerAssigneeID(); cfgAssignee != "" {
		return cfgAssignee
	}
	return os.Getenv("LINEAR_ASSIGNEE")
}

// newNotifier builds the chat-webhook notifier, preferring SLACK_WEBHOOK_URL
// and CONTRABASS_WEBHOOK_URL from the environment so endpoints stay out of
// committed workflow files.
func newNotifier(cfg *config.WorkflowConfig, logger *log.Logger) *notify.Notifier {
	slackURL := os.Getenv("SLACK_WEBHOOK_URL")
	if slackURL == "" && cfg != nil {
		slackURL = cfg.Notifications.SlackWebhookURL
	}
	webhookURL := os.Getenv("CONTRABASS_WEBHOOK_URL")
	if webhookURL == "" && cfg != nil {
		webhookURL = cfg.Notifications.WebhookURL
	}

	var events []string
	if cfg != nil {
		events = cfg.Notifications.Events
	}

	return notify.New(notify.Config{
		SlackWebhookURL: slackURL,
		WebhookURL:      webhookURL,
		Events:          events,
		Logger:          logger,
	})
}

// jiraConfig assembles the Jira tracker config, preferring JIRA_EMAIL and
// JIRA_API_TOKEN from the environment so credentials stay out of committed
// workflow files.
func jiraConfig(cfg *config.WorkflowConfig) tracker.JiraConfig {
	email := os.Getenv("JIRA_EMAIL")
	if email == "" {
		email = cfg.Jira.Email
	}
	token := os.Getenv("JIRA_API_TOKEN")
	if token == "" {
		token = cfg.Jira.APIToken
	}

	return tracker.JiraConfig{
		BaseURL:              cfg.Jira.BaseURL,
		Email:                email,
		APIToken:             token,
		Project:              cfg.Jira.Project,
		JQL:                  cfg.Jira.JQL,
		AccountID:            cfg.Jira.AccountID,
		Labels:               cfg.Jira.Labels,
		TransitionInProgress: cfg.Jira.TransitionInProgress,
		TransitionDone:       cfg.Jira.TransitionDone,
		TransitionFailed:     cfg.Jira.TransitionFailed,
		PageSize:             cfg.Jira.PageSize,
	}
}
