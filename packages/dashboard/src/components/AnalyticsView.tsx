import { useCallback, useEffect, useState } from "react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { AnalyticsSnapshot } from "../types";
import { formatCompactNumber, formatDuration, formatNumber } from "../i18n/format";
import { zhCN } from "../i18n/messages";

interface AnalyticsViewProps {
  refreshMs?: number;
}

type Status = "loading" | "ready" | "unavailable" | "error";

const DASH = "—";

function successRate(succeeded: number, runs: number): string {
  if (runs <= 0) {
    return DASH;
  }
  return `${Math.round((succeeded / runs) * 100)}%`;
}

function avgDurationLabel(avgDurationMs: number): string {
  if (avgDurationMs <= 0) {
    return DASH;
  }
  return formatDuration(Math.round(avgDurationMs / 1000));
}

export function AnalyticsView({ refreshMs = 5000 }: AnalyticsViewProps) {
  const [data, setData] = useState<AnalyticsSnapshot | null>(null);
  const [status, setStatus] = useState<Status>("loading");
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const response = await fetch("/api/v1/analytics");
      if (response.status === 503) {
        setStatus("unavailable");
        setData(null);
        setError(null);
        return;
      }
      if (!response.ok) {
        const body = (await response.json().catch(() => ({}))) as {
          error?: string;
        };
        throw new Error(body.error || response.statusText);
      }
      const payload = (await response.json()) as AnalyticsSnapshot;
      setData(payload);
      setStatus("ready");
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      // Keep showing the last good payload if we have one.
      setStatus((prev) => (prev === "ready" ? prev : "error"));
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => {
      void load();
    }, refreshMs);
    return () => window.clearInterval(timer);
  }, [load, refreshMs]);

  if (status === "loading") {
    return (
      <EmptyPanel>
        <p className="text-sm text-muted-foreground">{zhCN.analytics.loading}</p>
      </EmptyPanel>
    );
  }

  if (status === "unavailable") {
    return (
      <EmptyPanel>
        <p className="text-sm font-medium text-foreground">
          {zhCN.analytics.unavailable}
        </p>
        <p className="mt-1 text-xs text-muted-foreground">
          {zhCN.analytics.unavailableHint}
        </p>
      </EmptyPanel>
    );
  }

  if (status === "error" || !data) {
    return (
      <EmptyPanel>
        <p className="text-sm text-destructive" role="alert">
          {zhCN.analytics.loadFailed(error ?? "unknown")}
        </p>
      </EmptyPanel>
    );
  }

  const agents = Object.entries(data.by_agent ?? {}).sort(([a], [b]) =>
    a.localeCompare(b),
  );

  return (
    <div className="min-h-0 overflow-auto" aria-label={zhCN.analytics.ariaLabel}>
      <div className="grid gap-4 pb-6">
        {error ? (
          <p
            className="rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive"
            role="alert"
          >
            {zhCN.analytics.loadFailed(error)}
          </p>
        ) : null}

        <section
          className="grid grid-cols-2 gap-3 lg:grid-cols-4"
          aria-label={zhCN.analytics.ariaLabel}
        >
          <SummaryCard
            label={zhCN.analytics.cards.totalRuns}
            value={formatNumber(data.total_runs)}
          />
          <SummaryCard
            label={zhCN.analytics.cards.successRate}
            value={successRate(data.succeeded, data.total_runs)}
            subtle={`${formatNumber(data.succeeded)} 成功 / ${formatNumber(data.failed)} 失败`}
          />
          <SummaryCard
            label={zhCN.analytics.cards.tokens}
            value={zhCN.metrics.tokensInOut(
              formatCompactNumber(data.tokens_in),
              formatCompactNumber(data.tokens_out),
            )}
          />
          <SummaryCard
            label={zhCN.analytics.cards.avgDuration}
            value={avgDurationLabel(data.avg_duration_ms)}
          />
        </section>

        <section className="overflow-hidden rounded-lg border border-border/70 bg-card/80 shadow-sm">
          {agents.length === 0 ? (
            <p className="px-4 py-8 text-center text-sm text-muted-foreground">
              {zhCN.analytics.empty}
            </p>
          ) : (
            <Table aria-label={zhCN.analytics.tableAriaLabel}>
              <TableHeader>
                <TableRow>
                  <TableHead>{zhCN.analytics.headers.agentType}</TableHead>
                  <TableHead className="text-right">
                    {zhCN.analytics.headers.runs}
                  </TableHead>
                  <TableHead className="text-right">
                    {zhCN.analytics.headers.succeeded}
                  </TableHead>
                  <TableHead className="text-right">
                    {zhCN.analytics.headers.failed}
                  </TableHead>
                  <TableHead className="text-right">
                    {zhCN.analytics.headers.successRate}
                  </TableHead>
                  <TableHead className="text-right">
                    {zhCN.analytics.headers.avgDuration}
                  </TableHead>
                  <TableHead className="text-right">
                    {zhCN.analytics.headers.tokensIn}
                  </TableHead>
                  <TableHead className="text-right">
                    {zhCN.analytics.headers.tokensOut}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {agents.map(([agentType, agent]) => (
                  <TableRow key={agentType}>
                    <TableCell className="font-mono text-xs text-foreground">
                      {agentType}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs tabular-nums">
                      {formatNumber(agent.runs)}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs tabular-nums">
                      {formatNumber(agent.succeeded)}
                    </TableCell>
                    <TableCell
                      className={`text-right font-mono text-xs tabular-nums ${
                        agent.failed > 0 ? "text-destructive" : ""
                      }`}
                    >
                      {formatNumber(agent.failed)}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs tabular-nums">
                      {successRate(agent.succeeded, agent.runs)}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs tabular-nums">
                      {avgDurationLabel(agent.avg_duration_ms)}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs tabular-nums">
                      {formatCompactNumber(agent.tokens_in)}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs tabular-nums">
                      {formatCompactNumber(agent.tokens_out)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </section>
      </div>
    </div>
  );
}

function SummaryCard({
  label,
  value,
  subtle,
}: {
  label: string;
  value: string | number;
  subtle?: string;
}) {
  return (
    <div className="rounded-lg border border-border/70 bg-card/80 px-4 py-3 shadow-sm">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <p
        className="mt-2 min-w-0 truncate font-mono text-lg font-semibold tabular-nums text-foreground"
        title={String(value)}
      >
        {value}
      </p>
      {subtle ? (
        <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
          {subtle}
        </p>
      ) : null}
    </div>
  );
}

function EmptyPanel({ children }: { children: React.ReactNode }) {
  return (
    <section className="flex min-h-44 flex-col items-center justify-center rounded-2xl border border-dashed border-border/70 bg-card/60 px-6 py-10 text-center">
      <div>{children}</div>
    </section>
  );
}
