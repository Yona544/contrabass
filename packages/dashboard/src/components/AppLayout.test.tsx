import { afterEach, describe, expect, it } from "bun:test";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { TooltipProvider } from "@/components/ui/tooltip";
import type { StateSnapshot } from "../types";
import { AppLayout } from "./AppLayout";

function expectInDocument(value: unknown) {
  (expect(value) as any).toBeInTheDocument();
}

afterEach(() => {
  cleanup();
});

function makeState(): StateSnapshot {
  return {
    stats: {
      Running: 0,
      MaxAgents: 4,
      TotalTokensIn: 0,
      TotalTokensOut: 0,
      StartTime: "2026-05-14T12:00:00Z",
      PollCount: 3,
    },
    running: [],
    backoff: [],
    issues: {},
    generated_at: "2026-05-14T12:05:00Z",
    runtime: {
      model_name: "gpt-5-codex",
      project_url: "https://linear.app/example/project/contrabass",
      tracker_type: "internal",
      tracker_scope: ".contrabass/runtime-board",
      agent_type: "codex",
      max_concurrency: 4,
      poll_interval_ms: 1500,
      max_retry_backoff_ms: 45000,
      agent_timeout_ms: 120000,
      stall_timeout_ms: 30000,
      workspace_base_dir: "worktrees",
      hooks: {
        before_run: true,
        after_run: false,
        before_remove: true,
      },
      team: {
        max_workers: 3,
        max_fix_loops: 2,
        claim_lease_seconds: 90,
        state_dir: ".contrabass/team-runtime",
        execution_mode: "team",
        worker_mode: "goroutine",
      },
      linear: {
        issue_details_enabled: false,
        sync_comments_enabled: false,
        sync_comments_mode: "reply_thread",
      },
    },
  };
}

describe("AppLayout", () => {
  it("switches from queue tables to runtime settings", () => {
    render(
      <TooltipProvider>
        <AppLayout
          state={makeState()}
          connected={true}
          runtimeLabel="5分钟"
        />
      </TooltipProvider>,
    );

    expectInDocument(screen.getByText("暂无运行中任务"));

    fireEvent.click(screen.getByRole("button", { name: "设置" }));

    expectInDocument(screen.getByRole("heading", { name: "运行设置" }));
    expectInDocument(screen.getByText("gpt-5-codex"));
    expect(screen.queryByText("暂无运行中任务")).toBeNull();
  });
});
