package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/log"

	contrabass "github.com/junhoyeo/contrabass"
	"github.com/junhoyeo/contrabass/internal/config"
	"github.com/junhoyeo/contrabass/internal/history"
	"github.com/junhoyeo/contrabass/internal/hub"
	"github.com/junhoyeo/contrabass/internal/notify"
	"github.com/junhoyeo/contrabass/internal/orchestrator"
	"github.com/junhoyeo/contrabass/internal/schedule"
	"github.com/junhoyeo/contrabass/internal/tracker"
	"github.com/junhoyeo/contrabass/internal/tui"
	"github.com/junhoyeo/contrabass/internal/types"
	"github.com/junhoyeo/contrabass/internal/web"
)

const teamEventBufferSize = 256

var (
	startTUITeamEventBridge = tui.StartTeamEventBridge
	dispatchRootBoardIssues = dispatchBoardIssues
	runRootTeamIssue        = runTeamWithHooks
	startTeamWebServer      = runTeamExecutionWebServer
)

// teamDispatchController implements web.DispatchController for team mode,
// where there is no orchestrator: pausing simply stops the board dispatch
// loop from picking up new issues. Team mode has no backoff queue, so
// RetryNow always reports not-found.
type teamDispatchController struct {
	paused atomic.Bool
}

func (c *teamDispatchController) SetDispatchPaused(paused bool) { c.paused.Store(paused) }
func (c *teamDispatchController) DispatchPaused() bool          { return c.paused.Load() }
func (c *teamDispatchController) RetryNow(string) bool          { return false }

// pausableSnapshotProvider surfaces the team controller's paused state in
// dashboard snapshots.
type pausableSnapshotProvider struct {
	inner      web.SnapshotProvider
	controller *teamDispatchController
}

func (p pausableSnapshotProvider) Snapshot() orchestrator.StateSnapshot {
	snapshot := p.inner.Snapshot()
	snapshot.DispatchPaused = p.controller.DispatchPaused()
	return snapshot
}

func runTeamExecutionApp(
	ctx context.Context,
	cfgPath string,
	watcher *config.Watcher,
	logger *log.Logger,
	noTUI bool,
	dryRun bool,
	webOpts webOptions,
) error {
	if watcher == nil {
		return errors.New("config watcher is required for team execution")
	}

	controller := &teamDispatchController{}

	var webSink chan<- web.WebEvent
	if webOpts.Enabled {
		var err error
		webSink, err = startTeamWebServer(ctx, logger, webOpts, controller, watcher.GetConfig())
		if err != nil {
			return err
		}
	}

	notifier := newNotifier(watcher.GetConfig(), logger)
	if notifier.Enabled() {
		go notifier.Start(ctx)
	}

	gate, err := buildScheduleGate(watcher.GetConfig())
	if err != nil {
		return err
	}

	forwardToWeb := func(teamEvents <-chan types.TeamEvent) <-chan types.TeamEvent {
		if webSink == nil {
			return teamEvents
		}
		out := make(chan types.TeamEvent, teamEventBufferSize)
		go func() {
			defer close(out)
			for evt := range teamEvents {
				webSink <- web.NewTeamWebEvent(evt)
				out <- evt
			}
		}()
		return out
	}

	if dryRun {
		return runTeamExecutionLoop(ctx, cfgPath, watcher, nil, notifier, gate, controller, true)
	}

	if noTUI {
		return runTeamExecutionLoop(ctx, cfgPath, watcher, nil, notifier, gate, controller, false)
	}

	teamEvents := make(chan types.TeamEvent, teamEventBufferSize)
	cfg := watcher.GetConfig()
	return runTeamTUI(ctx, cfg, forwardToWeb(teamEvents), func(runCtx context.Context) error {
		defer close(teamEvents)
		return runTeamExecutionLoop(runCtx, cfgPath, watcher, teamEvents, notifier, gate, controller, false)
	})
}

func runTeamExecutionWebServer(
	ctx context.Context,
	logger *log.Logger,
	webOpts webOptions,
	controller *teamDispatchController,
	cfg *config.WorkflowConfig,
) (chan<- web.WebEvent, error) {
	webEvents := make(chan web.WebEvent, 256)
	h := hub.NewHub(webEvents)
	go h.Run(ctx)

	dashboardFS, err := fs.Sub(contrabass.DashboardDistFS, "packages/dashboard/dist")
	if err != nil {
		return nil, fmt.Errorf("sub dashboard dist fs: %w", err)
	}

	var provider web.SnapshotProvider = web.NewTeamSnapshotProvider()
	if controller != nil {
		provider = pausableSnapshotProvider{inner: provider, controller: controller}
	}
	srv := web.NewServer(webOpts.ListenAddr, provider, h, dashboardFS)
	srv.SetAuthToken(webOpts.AuthToken)
	if controller != nil {
		srv.SetDispatchController(controller)
	}
	if cfg.HistoryEnabled() {
		// Read-only: team runs do not record history, but past
		// orchestrator-mode runs in the same repo stay visible.
		srv.SetHistoryProvider(history.NewStore(cfg.HistoryDir()))
	}

	listener, err := net.Listen("tcp", srv.ListenAddr())
	if err != nil {
		return nil, fmt.Errorf("listen web dashboard: %w", err)
	}

	go func() {
		if serveErr := srv.Serve(ctx, listener); serveErr != nil && logger != nil {
			logger.Error("web server error", "err", serveErr)
		}
	}()

	fmt.Fprintf(os.Stderr, "Web dashboard available at %s\n", webOpts.dashboardURL())
	return webEvents, nil
}
func runTeamExecutionLoop(
	ctx context.Context,
	cfgPath string,
	watcher *config.Watcher,
	teamEvents chan<- types.TeamEvent,
	notifier *notify.Notifier,
	gate *schedule.Schedule,
	controller *teamDispatchController,
	singlePoll bool,
) error {
	for {
		cfg := watcher.GetConfig()
		if cfg == nil {
			return errors.New("workflow config is unavailable")
		}
		if err := validateTeamExecutionConfig(cfg); err != nil {
			return err
		}

		if gate != nil {
			if summary, closed := gate.Tick(time.Now()); closed {
				notifier.Notify(web.NewScheduleWebEvent(summary))
			}
		}

		hooks := teamRunHooks{
			ParentContext:        ctx,
			DisableSignalHandler: true,
		}
		if teamEvents != nil {
			hooks.EventHandlers = append(hooks.EventHandlers, func(_ context.Context, event types.TeamEvent) {
				select {
				case <-ctx.Done():
				case teamEvents <- event:
				}
			})
		}
		if notifier.Enabled() {
			hooks.EventHandlers = append(hooks.EventHandlers, func(_ context.Context, event types.TeamEvent) {
				notifier.Notify(web.NewTeamWebEvent(event))
			})
		}

		dispatchOpts := boardDispatchOptions{
			ConfigPath: cfgPath,
			UntilEmpty: true,
		}
		dispatchOpts.ContinueDispatch = func() (bool, string) {
			if controller != nil && controller.DispatchPaused() {
				return false, "dispatch paused via control plane"
			}
			if gate != nil {
				return gate.AllowDispatch(time.Now())
			}
			return true, ""
		}
		runIssue := func(opts teamRunOptions) error {
			return runRootTeamIssue(opts, hooks)
		}
		if gate != nil {
			inner := runIssue
			runIssue = func(opts teamRunOptions) error {
				gate.RecordStart(time.Now())
				runErr := inner(opts)
				// Team runs do not surface token counts at this layer, so
				// only the issue budget advances in team mode.
				gate.RecordCompletion(runErr == nil, 0, 0)
				return runErr
			}
		}

		if err := dispatchRootBoardIssues(
			ctx,
			io.Discard,
			newLocalBoardTracker(cfg),
			dispatchOpts,
			runIssue,
		); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		if singlePoll {
			return nil
		}

		pollInterval := time.Duration(cfg.PollIntervalMs()) * time.Millisecond
		if pollInterval <= 0 {
			pollInterval = time.Second
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(pollInterval):
		}
	}
}

func runTeamTUI(
	ctx context.Context,
	cfg *config.WorkflowConfig,
	teamEvents <-chan types.TeamEvent,
	runner func(context.Context) error,
) error {
	tuiCtx, tuiCancel := context.WithCancel(ctx)
	defer tuiCancel()

	model := tui.NewModel()
	p := tea.NewProgram(withViewportProgramOptions(model))

	statusEvents := make(chan orchestrator.OrchestratorEvent, 1)
	statusEvents <- teamExecutionStatusEvent(cfg)
	close(statusEvents)

	startTUIEventBridge(tuiCtx, p, statusEvents)
	startTUITeamEventBridge(tuiCtx, p, teamEvents)

	runDone := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				runDone <- fmt.Errorf("team runtime panic: %v", r)
			}
		}()
		runDone <- runner(tuiCtx)
	}()

	_, tuiErr := runTUIProgram(p)
	tui.CleanupNativeImage()

	tuiCancel()
	select {
	case runErr := <-runDone:
		if runErr != nil {
			if tuiErr != nil {
				return fmt.Errorf("team runtime failed: %w (tui error: %v)", runErr, tuiErr)
			}
			return runErr
		}
	case <-time.After(runTUIShutdownTimeout):
		if tuiErr != nil {
			return fmt.Errorf("timed out waiting for team runtime shutdown: %w", tuiErr)
		}
		return errors.New("timed out waiting for team runtime shutdown")
	}

	return tuiErr
}

func validateTeamExecutionConfig(cfg *config.WorkflowConfig) error {
	if cfg == nil {
		return errors.New("workflow config is nil")
	}

	switch cfg.TrackerType() {
	case "internal", "local":
		return nil
	default:
		return fmt.Errorf(
			"team execution requires tracker.type internal/local, got %q",
			cfg.TrackerType(),
		)
	}
}

func newLocalBoardTracker(cfg *config.WorkflowConfig) *tracker.LocalTracker {
	actor := os.Getenv("TRACKER_ACTOR")
	if actor == "" && cfg != nil {
		actor = cfg.GitHubAssignee()
	}

	return tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    cfg.LocalBoardDir(),
		IssuePrefix: cfg.LocalIssuePrefix(),
		Actor:       actor,
	})
}

func teamExecutionStatusEvent(cfg *config.WorkflowConfig) orchestrator.OrchestratorEvent {
	if cfg == nil {
		cfg = &config.WorkflowConfig{}
	}

	modelName, _ := cfg.Model()
	projectURL := cfg.TrackerProjectURL()
	trackerType := cfg.TrackerType()
	trackerScope := projectURL
	if trackerType == "internal" || trackerType == "local" {
		trackerScope = cfg.LocalBoardDir()
	}

	return orchestrator.OrchestratorEvent{
		Type: orchestrator.EventStatusUpdate,
		Data: orchestrator.StatusUpdate{
			Stats: orchestrator.Stats{
				MaxAgents: cfg.TeamMaxWorkers(),
				StartTime: time.Now(),
			},
			ModelName:    modelName,
			ProjectURL:   projectURL,
			TrackerType:  trackerType,
			TrackerScope: trackerScope,
		},
	}
}
