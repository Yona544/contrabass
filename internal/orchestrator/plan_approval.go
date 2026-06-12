package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/junhoyeo/contrabass/internal/approval"
	"github.com/junhoyeo/contrabass/internal/logging"
	"github.com/junhoyeo/contrabass/internal/timeline"
	"github.com/junhoyeo/contrabass/internal/types"
)

const (
	planFileName    = "PLAN.md"
	maxPlanBytes    = 64 * 1024
	planOnlySuffix  = "\n\n---\nPLANNING MODE — read the codebase but DO NOT implement anything yet.\nWrite a concrete, numbered implementation plan (files to touch, approach,\nrisks, how to verify) to a file named PLAN.md in the working directory,\nthen stop. A human reviews the plan before the implementation run."
	approvedPrefix  = "\n\n---\n## Approved plan\nFollow this previously approved plan:\n\n"
	missingPlanNote = "(the planning run finished without writing PLAN.md — approve to proceed anyway, or reset to re-plan)"
)

// SetApprovalStore enables the plan-approval gate: issues without approval
// state run in planning mode first and park until approved.
func (o *Orchestrator) SetApprovalStore(store *approval.Store) {
	o.approvals = store
}

// ApproveIssue clears a parked issue for execution (used by the web control
// plane; the CLI writes the shared state directory directly).
func (o *Orchestrator) ApproveIssue(issueID string) error {
	if o.approvals == nil {
		return fmt.Errorf("plan approval is not enabled")
	}
	if err := o.approvals.Approve(issueID); err != nil {
		return err
	}
	logging.LogIssueEvent(o.logger, issueID, "plan_approved")
	return nil
}

// approvalParked reports whether dispatch must skip the issue because its
// plan awaits human approval.
func (o *Orchestrator) approvalParked(issueID string) bool {
	if o.approvals == nil {
		return false
	}
	status, _, err := o.approvals.Get(issueID)
	if err != nil {
		o.logger.Warn("approval state read failed", "issue_id", issueID, "err", err)
		return false
	}
	return status == approval.StatusPlanned
}

// applyApprovalPrompt adapts the rendered prompt for the approval gate:
// unplanned issues get planning-mode instructions (and the run is marked
// plan-only), approved issues get their plan attached.
func (o *Orchestrator) applyApprovalPrompt(issueID, prompt string) (string, bool) {
	if o.approvals == nil {
		return prompt, false
	}

	status, plan, err := o.approvals.Get(issueID)
	if err != nil {
		o.logger.Warn("approval state read failed", "issue_id", issueID, "err", err)
		return prompt, false
	}

	switch status {
	case approval.StatusNone:
		return prompt + planOnlySuffix, true
	case approval.StatusApproved:
		if strings.TrimSpace(plan) != "" {
			return prompt + approvedPrefix + plan, false
		}
		return prompt, false
	default:
		return prompt, false
	}
}

// finishPlanRun handles the successful end of a planning run: capture
// PLAN.md before the workspace disappears, park the issue, and ask for
// approval. Branch-advance verification and auto-PR do not apply — a plan
// is not a code change.
func (o *Orchestrator) finishPlanRun(ctx context.Context, entry *runEntry, attempt types.RunAttempt) {
	issue := entry.issue
	plan := readPlanFile(entry.workspace)
	if plan == "" {
		plan = missingPlanNote
	}

	if err := o.approvals.MarkPlanned(issue.ID, plan); err != nil {
		logging.LogIssueEvent(o.logger, issue.ID, "plan_record_failed", "err", err)
	}

	o.recordTimelineNode(ctx, issue, attempt,
		"plan-awaiting-approval", timeline.NodeStatusSucceeded, "Plan awaiting approval",
		"The planning run finished; the issue is parked until the plan is approved.", "", true)

	comment := fmt.Sprintf(
		"Plan for approval (attempt %d):\n\n%s\n\nApprove with: contrabass approve %s",
		attempt.Attempt, plan, issue.ID,
	)
	if o.shouldSuppressLegacyComment() {
		logging.LogIssueEvent(o.logger, issue.ID, "legacy_comment_suppressed", "reason", "linear_sync_enabled")
	} else if err := o.tracker.PostComment(ctx, issue.ID, comment); err != nil {
		logging.LogIssueEvent(o.logger, issue.ID, "post_comment_failed", "err", err)
	}

	o.runAfterRunHook(ctx, issue.ID, issue, attempt)
	if err := o.workspace.Cleanup(ctx, issue.ID); err != nil {
		logging.LogIssueEvent(o.logger, issue.ID, "workspace_cleanup_failed", "stage", "plan_run", "err", err)
	}

	// Back to Unclaimed: visible on the board, parked by approvalParked
	// until a human approves.
	if err := o.tracker.UpdateIssueState(ctx, issue.ID, types.Unclaimed); err != nil {
		logging.LogIssueEvent(o.logger, issue.ID, "plan_unclaim_failed", "err", err)
	}

	logging.LogAgentEvent(o.logger, issue.ID, "plan_finished", "status", attempt.Phase.String())
}

func readPlanFile(workspace string) string {
	data, err := os.ReadFile(filepath.Join(workspace, planFileName))
	if err != nil {
		return ""
	}
	if len(data) > maxPlanBytes {
		data = data[:maxPlanBytes]
	}
	return strings.TrimSpace(string(data))
}
