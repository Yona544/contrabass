import { afterEach, describe, expect, it, mock } from "bun:test";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import type { Issue, SheetData } from "../types";
import { IssueDetailSheet } from "./IssueDetailSheet";

function expectInDocument(value: unknown) {
  (expect(value) as any).toBeInTheDocument();
}

function baseIssue(overrides: Partial<Issue> = {}): Issue {
  return {
    id: "issue-1",
    identifier: "ZII-1",
    title: "Base issue title",
    description: "Base description stays visible",
    state: 0,
    priority: 2,
    labels: ["dashboard"],
    url: "https://linear.app/acme/issue/ZII-1",
    branch_name: "symphony/zii-1",
    blocked_by: [],
    created_at: "2026-03-05T10:00:00.000Z",
    updated_at: "2026-03-05T10:05:00.000Z",
    tracker_meta: { linear_state: "Todo" },
    ...overrides,
  };
}

function sheetData(issue = baseIssue()): SheetData {
  return { kind: "todo", issue };
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

afterEach(() => {
  cleanup();
  mock.restore();
});

describe("IssueDetailSheet", () => {
  it("loads issue details while preserving base snapshot data during loading", async () => {
    const { fetchMock, restore } = installFetchMock((url) => {
      if (url.endsWith("/details")) {
        return jsonResponse({
          issue: { ...baseIssue(), title: "Loaded issue title" },
          linear: {
            assignee: { id: "user-1", display_name: "Ada Lovelace" },
            creator: { id: "user-2", display_name: "Grace Hopper" },
            team: { id: "team-1", name: "Ziikoo" },
            project: { id: "project-1", name: "Contrabass" },
            cycle: { id: "cycle-1", name: "May Sprint", number: 7 },
            estimate: 3,
            due_date: "2026-05-20",
            relations: [
              {
                type: "blocks",
                direction: "inverse",
                issue: { id: "issue-0", identifier: "ZII-0" },
              },
            ],
            fetched_at: "2026-03-05T10:06:00.000Z",
          },
          generated_at: "2026-03-05T10:06:00.000Z",
        });
      }
      return jsonResponse({
        issue_id: "issue-1",
        runs: [],
        nodes: [],
        run_sync_states: [],
        node_sync_states: [],
        generated_at: "2026-03-05T10:06:00.000Z",
      });
    });

    try {
      render(<IssueDetailSheet data={sheetData()} onOpenChange={() => {}} />);

      expectInDocument(screen.getByText("Base issue title"));
      expectInDocument(screen.getByText("Base description stays visible"));

      await waitFor(() => {
        expectInDocument(screen.getByText("Loaded issue title"));
        expectInDocument(screen.getByText("Ada Lovelace"));
      });
      expectInDocument(screen.getByText("Grace Hopper"));
      expectInDocument(screen.getByText("Contrabass"));
      expectInDocument(screen.getByText(/ZII-0/));
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/issues/issue-1/details",
        expect.any(Object),
      );
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/issues/issue-1/timeline",
        expect.any(Object),
      );
    } finally {
      restore();
    }
  });

  it("renders workflow timeline rows and sync badges", async () => {
    const { restore } = installFetchMock((url) => {
      if (url.endsWith("/details")) {
        return jsonResponse({
          issue: baseIssue(),
          generated_at: "2026-03-05T10:06:00.000Z",
        });
      }
      return jsonResponse({
        issue_id: "issue-1",
        runs: [{ run_id: "run-1", attempt: 1, status: "completed" }],
        nodes: [
          {
            run_id: "run-1",
            node_id: "complete",
            status: "completed",
            title: "Run completed",
            summary: "All checks passed",
            completed_at: "2026-03-05T10:06:00.000Z",
          },
        ],
        run_sync_states: [],
        node_sync_states: [
          {
            run_id: "run-1",
            node_id: "complete",
            target: "linear",
            status: "synced",
          },
        ],
        generated_at: "2026-03-05T10:06:00.000Z",
      });
    });

    try {
      render(<IssueDetailSheet data={sheetData()} onOpenChange={() => {}} />);

      await waitFor(() => {
        expectInDocument(screen.getByText("Run completed"));
        expectInDocument(screen.getByText("All checks passed"));
        expectInDocument(screen.getByText("linear: synced"));
      });
    } finally {
      restore();
    }
  });

  it("shows inline non-blocking errors for failed detail and timeline fetches", async () => {
    const { restore } = installFetchMock((url) => {
      if (url.endsWith("/details")) {
        return jsonResponse({ error: "provider unavailable" }, 503);
      }
      return jsonResponse({ error: "timeline unavailable" }, 503);
    });

    try {
      render(<IssueDetailSheet data={sheetData()} onOpenChange={() => {}} />);

      expectInDocument(screen.getByText("Base issue title"));
      await waitFor(() => {
        expectInDocument(
          screen.getByText("详情加载失败：provider unavailable"),
        );
        expectInDocument(
          screen.getByText("时间线加载失败：timeline unavailable"),
        );
      });
    } finally {
      restore();
    }
  });
});
