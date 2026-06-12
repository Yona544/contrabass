import { afterEach, describe, expect, it, mock } from "bun:test";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import type { AnalyticsSnapshot } from "../types";
import { AnalyticsView } from "./AnalyticsView";
import { zhCN } from "../i18n/messages";

function expectInDocument(value: unknown) {
  (expect(value) as any).toBeInTheDocument();
}

function jsonResponse(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

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

function analyticsPayload(): AnalyticsSnapshot {
  return {
    total_runs: 10,
    succeeded: 8,
    failed: 2,
    tokens_in: 1234,
    tokens_out: 567,
    avg_duration_ms: 65000,
    cost_usd: 12.345,
    unpriced_runs: 2,
    by_agent: {
      claude: {
        runs: 6,
        succeeded: 6,
        failed: 0,
        tokens_in: 1000,
        tokens_out: 400,
        avg_duration_ms: 60000,
        cost_usd: 8.5,
      },
      codex: {
        runs: 4,
        succeeded: 2,
        failed: 2,
        tokens_in: 234,
        tokens_out: 167,
        avg_duration_ms: 72500,
        cost_usd: 3.845,
      },
    },
    generated_at: "2026-06-12T08:00:00Z",
  };
}

afterEach(() => {
  cleanup();
  mock.restore();
});

describe("AnalyticsView", () => {
  it("renders summary cards and the per-agent table from the analytics payload", async () => {
    const { fetchMock, restore } = installFetchMock(() =>
      jsonResponse(analyticsPayload()),
    );

    try {
      render(<AnalyticsView />);

      await waitFor(() => {
        expectInDocument(screen.getByText(zhCN.analytics.cards.totalRuns));
      });

      // Summary cards
      expectInDocument(screen.getByText("10"));
      expectInDocument(screen.getByText("80%"));
      expectInDocument(screen.getByText("1,234 输入 / 567 输出"));
      expectInDocument(screen.getByText("1分 5秒"));
      expectInDocument(screen.getByText("$12.35"));
      expectInDocument(screen.getByText(zhCN.analytics.costUnpriced(2)));

      // Per-agent table
      expectInDocument(screen.getByRole("table"));
      expectInDocument(screen.getByText(zhCN.analytics.headers.agentType));
      expectInDocument(screen.getByText("claude"));
      expectInDocument(screen.getByText("codex"));
      expectInDocument(screen.getByText("100%"));
      expectInDocument(screen.getByText("50%"));
      expectInDocument(screen.getByText("1分 0秒"));
      expectInDocument(screen.getByText("1分 13秒"));
      expectInDocument(screen.getByText(zhCN.analytics.headers.cost));
      expectInDocument(screen.getByText("$8.50"));
      expectInDocument(screen.getByText("$3.85"));

      expect(fetchMock).toHaveBeenCalledWith("/api/v1/analytics");
    } finally {
      restore();
    }
  });

  it("renders the informative empty state when analytics is disabled (503)", async () => {
    const { restore } = installFetchMock(() =>
      jsonResponse({ error: "history disabled" }, 503),
    );

    try {
      render(<AnalyticsView />);

      await waitFor(() => {
        expectInDocument(screen.getByText(zhCN.analytics.unavailable));
      });
      expectInDocument(screen.getByText(zhCN.analytics.unavailableHint));
      expect(screen.queryByRole("table")).toBeNull();
    } finally {
      restore();
    }
  });

  it("shows a non-fatal error state when the fetch fails", async () => {
    const { restore } = installFetchMock(() =>
      jsonResponse({ error: "kaput" }, 500),
    );

    try {
      render(<AnalyticsView />);

      await waitFor(() => {
        expectInDocument(
          screen.getByText(zhCN.analytics.loadFailed("kaput")),
        );
      });
    } finally {
      restore();
    }
  });

  it("renders the empty table hint when there are no agent rows", async () => {
    const { restore } = installFetchMock(() =>
      jsonResponse({
        ...analyticsPayload(),
        total_runs: 0,
        succeeded: 0,
        failed: 0,
        cost_usd: 0,
        unpriced_runs: 0,
        by_agent: {},
      }),
    );

    try {
      render(<AnalyticsView />);

      await waitFor(() => {
        expectInDocument(screen.getByText(zhCN.analytics.empty));
      });
      expect(screen.queryByRole("table")).toBeNull();
      // No unpriced note when every run is priced.
      expect(screen.queryByText(/未定价/)).toBeNull();
      expectInDocument(screen.getByText("$0.00"));
    } finally {
      restore();
    }
  });
});
