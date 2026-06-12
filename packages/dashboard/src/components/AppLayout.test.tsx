import { afterEach, describe, expect, it, mock } from "bun:test";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import "@testing-library/jest-dom";
import { TooltipProvider } from "@/components/ui/tooltip";
import type { StateSnapshot } from "../types";
import { AppLayout } from "./AppLayout";
import { zhCN } from "../i18n/messages";

function expectInDocument(value: unknown) {
  (expect(value) as any).toBeInTheDocument();
}

afterEach(() => {
  cleanup();
  mock.restore();
});

function installFetchMock(
  handler: (url: string) => Response | Promise<Response>,
) {
  const original = globalThis.fetch;
  const fetchMock = mock((input: RequestInfo | URL) => handler(String(input)));
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return {
    fetchMock,
    restore: () => {
      globalThis.fetch = original;
    },
  };
}

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

  it("mounts the retry actions panel on the backoff queue view", () => {
    const state = {
      ...makeState(),
      backoff: [
        {
          issue_id: "CB-9",
          attempt: 2,
          retry_at: "2026-05-14T12:30:00Z",
          error: "exit status 1",
        },
      ],
    };

    render(
      <TooltipProvider>
        <AppLayout state={state} connected={true} runtimeLabel="5分钟" />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "退避队列" }));

    expectInDocument(
      screen.getByRole("button", { name: zhCN.retryQueue.retryAria("CB-9") }),
    );
  });

  it("switches to the analytics view from the sidebar", async () => {
    const { fetchMock, restore } = installFetchMock(() =>
      new Response(JSON.stringify({ error: "history disabled" }), {
        status: 503,
        headers: { "Content-Type": "application/json" },
      }),
    );

    try {
      render(
        <TooltipProvider>
          <AppLayout
            state={makeState()}
            connected={true}
            runtimeLabel="5分钟"
          />
        </TooltipProvider>,
      );

      fireEvent.click(screen.getByRole("button", { name: "分析" }));

      expectInDocument(
        screen.getByRole("heading", { name: zhCN.analytics.title }),
      );
      await waitFor(() => {
        expectInDocument(screen.getByText(zhCN.analytics.unavailable));
      });
      expect(fetchMock).toHaveBeenCalledWith("/api/v1/analytics");
      expect(screen.queryByText("暂无运行中任务")).toBeNull();
    } finally {
      restore();
    }
  });

  it("surfaces the dispatch pause control from the state snapshot", () => {
    render(
      <TooltipProvider>
        <AppLayout
          state={{ ...makeState(), dispatch_paused: true }}
          connected={true}
          runtimeLabel="5分钟"
        />
      </TooltipProvider>,
    );

    expectInDocument(screen.getByText(zhCN.dispatch.pausedBadge));
    expectInDocument(
      screen.getByRole("button", { name: zhCN.dispatch.resume }),
    );
  });

  it("shows the pause action when dispatch is running", () => {
    render(
      <TooltipProvider>
        <AppLayout
          state={{ ...makeState(), dispatch_paused: false }}
          connected={true}
          runtimeLabel="5分钟"
        />
      </TooltipProvider>,
    );

    expectInDocument(
      screen.getByRole("button", { name: zhCN.dispatch.pause }),
    );
    expect(screen.queryByText(zhCN.dispatch.pausedBadge)).toBeNull();
  });
});
