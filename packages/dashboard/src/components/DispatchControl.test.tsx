import { afterEach, describe, expect, it, mock } from "bun:test";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import "@testing-library/jest-dom";
import { DispatchControl } from "./DispatchControl";
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
  handler: (url: string, init?: RequestInit) => Response | Promise<Response>,
) {
  const original = globalThis.fetch;
  const fetchMock = mock((input: RequestInfo | URL, init?: RequestInit) =>
    handler(String(input), init),
  );
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

describe("DispatchControl", () => {
  it("shows pause action when running and toggles to paused via the pause endpoint", async () => {
    const calls: Array<{ url: string; method?: string }> = [];
    const { restore } = installFetchMock((url, init) => {
      calls.push({ url, method: init?.method });
      return jsonResponse({ dispatch_paused: true });
    });

    try {
      render(<DispatchControl paused={false} />);

      expect(screen.queryByText(zhCN.dispatch.pausedBadge)).toBeNull();

      fireEvent.click(
        screen.getByRole("button", { name: zhCN.dispatch.pause }),
      );

      await waitFor(() => {
        expectInDocument(
          screen.getByRole("button", { name: zhCN.dispatch.resume }),
        );
      });

      expectInDocument(screen.getByText(zhCN.dispatch.pausedBadge));
      expect(calls).toEqual([
        { url: "/api/v1/control/pause", method: "POST" },
      ]);
    } finally {
      restore();
    }
  });

  it("shows loud paused state and resumes via the resume endpoint", async () => {
    const calls: Array<{ url: string; method?: string }> = [];
    const { restore } = installFetchMock((url, init) => {
      calls.push({ url, method: init?.method });
      return jsonResponse({ dispatch_paused: false });
    });

    try {
      render(<DispatchControl paused={true} />);

      expectInDocument(screen.getByText(zhCN.dispatch.pausedBadge));

      fireEvent.click(
        screen.getByRole("button", { name: zhCN.dispatch.resume }),
      );

      await waitFor(() => {
        expectInDocument(
          screen.getByRole("button", { name: zhCN.dispatch.pause }),
        );
      });

      expect(screen.queryByText(zhCN.dispatch.pausedBadge)).toBeNull();
      expect(calls).toEqual([
        { url: "/api/v1/control/resume", method: "POST" },
      ]);
    } finally {
      restore();
    }
  });

  it("disables the button while the toggle request is pending", async () => {
    let resolveToggle!: () => void;
    const togglePromise = new Promise<Response>((resolve) => {
      resolveToggle = () => resolve(jsonResponse({ dispatch_paused: true }));
    });

    const { restore } = installFetchMock(() => togglePromise);

    try {
      render(<DispatchControl paused={false} />);

      fireEvent.click(
        screen.getByRole("button", { name: zhCN.dispatch.pause }),
      );

      await waitFor(() => {
        (expect(
          screen.getByRole("button", { name: zhCN.dispatch.pending }),
        ) as any).toBeDisabled();
      });

      resolveToggle();

      await waitFor(() => {
        expectInDocument(
          screen.getByRole("button", { name: zhCN.dispatch.resume }),
        );
      });
    } finally {
      restore();
    }
  });

  it("hides the control when the control plane is not configured (503)", async () => {
    const { restore } = installFetchMock(() =>
      jsonResponse({ error: "control plane not configured" }, 503),
    );

    try {
      render(<DispatchControl paused={false} />);

      fireEvent.click(
        screen.getByRole("button", { name: zhCN.dispatch.pause }),
      );

      await waitFor(() => {
        expect(screen.queryByRole("button")).toBeNull();
      });
    } finally {
      restore();
    }
  });

  it("surfaces non-503 failures without flipping state", async () => {
    const { restore } = installFetchMock(() =>
      jsonResponse({ error: "boom" }, 500),
    );

    try {
      render(<DispatchControl paused={false} />);

      fireEvent.click(
        screen.getByRole("button", { name: zhCN.dispatch.pause }),
      );

      await waitFor(() => {
        expectInDocument(screen.getByText(zhCN.dispatch.toggleFailed("boom")));
      });

      const button = screen.getByRole("button", { name: zhCN.dispatch.pause });
      (expect(button) as any).not.toBeDisabled();
      expect(screen.queryByText(zhCN.dispatch.pausedBadge)).toBeNull();
    } finally {
      restore();
    }
  });
});
