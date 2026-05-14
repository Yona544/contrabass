import { afterEach, describe, expect, it } from "bun:test";
import { cleanup, render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import type { StateSnapshot } from "../types";
import { RuntimeSettings } from "./RuntimeSettings";

function expectInDocument(value: unknown) {
  (expect(value) as any).toBeInTheDocument();
}

afterEach(() => {
  cleanup();
});

function makeState(): StateSnapshot {
  return {
    stats: {
      Running: 2,
      MaxAgents: 4,
      TotalTokensIn: 1200,
      TotalTokensOut: 800,
      StartTime: "2026-05-14T12:00:00Z",
      PollCount: 7,
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
        after_run: true,
        before_remove: false,
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

describe("RuntimeSettings", () => {
  it("renders runtime, tracker, hook, and team diagnostics", () => {
    render(
      <RuntimeSettings
        state={makeState()}
        connected={true}
        runtimeLabel="5分钟"
      />,
    );

    expectInDocument(screen.getByText("在线"));
    expectInDocument(screen.getByText("5分钟"));
    expectInDocument(screen.getByText("gpt-5-codex"));
    expectInDocument(screen.getByText("internal"));
    expectInDocument(screen.getByText(".contrabass/runtime-board"));
    expectInDocument(screen.getByText("codex"));
    expectInDocument(screen.getByText("worktrees"));
    expectInDocument(screen.getByText("team"));
    expectInDocument(screen.getByText("goroutine"));
    expectInDocument(screen.getByText(".contrabass/team-runtime"));
    expectInDocument(screen.getByText("before_run"));
    expectInDocument(screen.getByText("after_run"));
    expectInDocument(screen.getByText("before_remove"));
    expect(screen.getAllByText("已配置")).toHaveLength(2);
    expectInDocument(screen.getByText("未配置"));
  });
});
