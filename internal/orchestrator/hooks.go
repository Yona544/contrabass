package orchestrator

import (
	"context"
	"strconv"

	"github.com/junhoyeo/contrabass/internal/hooks"
	"github.com/junhoyeo/contrabass/internal/logging"
	"github.com/junhoyeo/contrabass/internal/types"
)

func (o *Orchestrator) runWorkflowHook(
	ctx context.Context,
	name string,
	command string,
	issue types.Issue,
	attempt types.RunAttempt,
) error {
	return hooks.Run(ctx, hooks.Options{
		Name:    name,
		Command: command,
		Dir:     attempt.WorkspacePath,
		Env: map[string]string{
			"CONTRABASS_HOOK":             name,
			"CONTRABASS_ISSUE_ID":         issue.ID,
			"CONTRABASS_ISSUE_IDENTIFIER": issue.Identifier,
			"CONTRABASS_ISSUE_TITLE":      issue.Title,
			"CONTRABASS_ISSUE_URL":        issue.URL,
			"CONTRABASS_WORKSPACE":        attempt.WorkspacePath,
			"CONTRABASS_RUN_ATTEMPT":      strconv.Itoa(attempt.Attempt),
			"CONTRABASS_RUN_PHASE":        attempt.Phase.String(),
			"CONTRABASS_RUN_ERROR":        attempt.Error,
		},
	})
}

func (o *Orchestrator) runAfterRunHook(ctx context.Context, issueID string, issue types.Issue, attempt types.RunAttempt) {
	if err := o.runWorkflowHook(ctx, "after_run", o.currentConfig().HookAfterRun(), issue, attempt); err != nil {
		logging.LogIssueEvent(o.logger, issueID, "after_run_hook_failed", "err", err)
	}
}
