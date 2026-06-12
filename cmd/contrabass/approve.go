package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/junhoyeo/contrabass/internal/approval"
	"github.com/junhoyeo/contrabass/internal/config"
)

func newApproveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve [issue-id]",
		Short: "Approve a parked plan for execution",
		Long: `Approve an issue whose plan is awaiting review (approval.require_plan).
A running orchestrator picks the approval up on its next poll and dispatches
the implementation run. Approving an issue that has not planned yet skips
the planning run entirely.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runApprove,
	}
	cmd.Flags().String("config", "", "workflow file resolving approval.dir (default: WORKFLOW.md when present)")
	cmd.Flags().String("dir", "", "approval state directory override")
	cmd.Flags().Bool("list", false, "list plans awaiting approval")
	cmd.Flags().Bool("reset", false, "clear the issue's approval state so it plans again")
	return cmd
}

func runApprove(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")
	dir, _ := cmd.Flags().GetString("dir")
	list, _ := cmd.Flags().GetBool("list")
	reset, _ := cmd.Flags().GetBool("reset")

	if dir == "" {
		base, err := loadBaseConfig(configPath, "")
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		dir = base.ApprovalDir()
	}
	store := approval.NewStore(dir)
	out := cmd.OutOrStdout()

	if list {
		pending, err := store.Pending()
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			fmt.Fprintln(out, "no plans awaiting approval")
			return nil
		}
		for _, entry := range pending {
			excerpt := strings.SplitN(entry.Plan, "\n", 2)[0]
			if len(excerpt) > 100 {
				excerpt = excerpt[:97] + "..."
			}
			fmt.Fprintf(out, "%s  (planned %s)  %s\n",
				entry.IssueID, entry.UpdatedAt.Format("2006-01-02 15:04"), excerpt)
		}
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("issue id is required (or use --list)")
	}
	issueID := strings.TrimSpace(args[0])

	if reset {
		if err := store.Reset(issueID); err != nil {
			return err
		}
		fmt.Fprintf(out, "approval state for %s cleared; the next dispatch plans again\n", issueID)
		return nil
	}

	status, plan, err := store.Get(issueID)
	if err != nil {
		return err
	}
	if err := store.Approve(issueID); err != nil {
		return err
	}

	switch status {
	case approval.StatusPlanned:
		fmt.Fprintf(out, "approved %s — the orchestrator will run the plan on its next poll\n", issueID)
		if strings.TrimSpace(plan) != "" {
			fmt.Fprintf(out, "\n%s\n", plan)
		}
	case approval.StatusApproved:
		fmt.Fprintf(out, "%s was already approved\n", issueID)
	default:
		fmt.Fprintf(out, "pre-approved %s — the planning run will be skipped\n", issueID)
	}
	return nil
}

// approvalStoreFor builds the store the orchestrator and web server share.
func approvalStoreFor(cfg *config.WorkflowConfig) *approval.Store {
	if cfg == nil || !cfg.Approval.RequirePlan {
		return nil
	}
	return approval.NewStore(cfg.ApprovalDir())
}
