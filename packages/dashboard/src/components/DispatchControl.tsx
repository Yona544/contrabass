import { useEffect, useState } from "react";
import { Pause, Play } from "lucide-react";
import { zhCN } from "../i18n/messages";

interface DispatchControlProps {
  paused: boolean;
}

/**
 * Global pause/resume toggle for the orchestrator dispatch loop.
 * `paused` comes from the SSE state snapshot (`dispatch_paused`); a local
 * optimistic override from the POST response bridges the gap until the next
 * snapshot refresh. When the control plane is not configured (503) the
 * control hides itself.
 */
export function DispatchControl({ paused }: DispatchControlProps) {
  const [pending, setPending] = useState(false);
  const [localPaused, setLocalPaused] = useState<boolean | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // The server snapshot is the source of truth: drop the optimistic
  // override whenever a fresh value arrives.
  useEffect(() => {
    setLocalPaused(null);
  }, [paused]);

  const effectivePaused = localPaused ?? paused;

  async function handleToggle() {
    setPending(true);
    setError(null);
    try {
      const response = await fetch(
        effectivePaused ? "/api/v1/control/resume" : "/api/v1/control/pause",
        { method: "POST" },
      );
      if (response.status === 503) {
        setUnavailable(true);
        return;
      }
      if (!response.ok) {
        const body = (await response.json().catch(() => ({}))) as {
          error?: string;
        };
        throw new Error(body.error || response.statusText);
      }
      const body = (await response.json().catch(() => ({}))) as {
        dispatch_paused?: boolean;
      };
      setLocalPaused(
        typeof body.dispatch_paused === "boolean"
          ? body.dispatch_paused
          : !effectivePaused,
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setPending(false);
    }
  }

  if (unavailable) {
    return null;
  }

  return (
    <div className="flex items-center gap-2">
      {effectivePaused ? (
        <span
          role="status"
          title={zhCN.dispatch.pausedHint}
          className="inline-flex items-center gap-1.5 rounded-full border border-destructive/60 bg-destructive/15 px-3 py-1 text-xs font-bold uppercase tracking-wide text-destructive shadow-xs"
        >
          <span
            className="h-2 w-2 animate-pulse rounded-full bg-destructive"
            aria-hidden
          />
          {zhCN.dispatch.pausedBadge}
        </span>
      ) : null}
      <button
        type="button"
        onClick={handleToggle}
        disabled={pending}
        className={`inline-flex h-8 items-center justify-center gap-1.5 rounded-lg border px-3 text-xs font-medium transition disabled:cursor-not-allowed disabled:opacity-60 ${
          effectivePaused
            ? "border-destructive bg-destructive text-destructive-foreground shadow-[0_0_18px_var(--destructive)] hover:bg-destructive/90"
            : "border-border/70 bg-card/80 text-foreground shadow-xs hover:bg-muted"
        }`}
      >
        {effectivePaused ? (
          <Play className="h-3.5 w-3.5" aria-hidden />
        ) : (
          <Pause className="h-3.5 w-3.5" aria-hidden />
        )}
        {pending
          ? zhCN.dispatch.pending
          : effectivePaused
            ? zhCN.dispatch.resume
            : zhCN.dispatch.pause}
      </button>
      {error ? (
        <span className="text-xs text-destructive" role="alert">
          {zhCN.dispatch.toggleFailed(error)}
        </span>
      ) : null}
    </div>
  );
}
