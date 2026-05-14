import {
  Activity,
  GitBranch,
  Network,
  SlidersHorizontal,
} from "lucide-react";
import type { ComponentType } from "react";
import { formatDateTime, formatNumber } from "../i18n/format";
import type { RuntimeConfig, StateSnapshot } from "../types";

interface RuntimeSettingsProps {
  state: StateSnapshot;
  connected: boolean;
  runtimeLabel: string;
}

type SettingRow = {
  label: string;
  value: string | number;
  mono?: boolean;
};

const DASH = "-";

export function RuntimeSettings({
  state,
  connected,
  runtimeLabel,
}: RuntimeSettingsProps) {
  const runtime = state.runtime;

  return (
    <div className="min-h-0 overflow-auto">
      <div className="grid gap-4 pb-6">
        <section
          className="grid grid-cols-2 gap-3 lg:grid-cols-4"
          aria-label="运行诊断"
        >
          <StatusTile
            label="连接"
            value={connected ? "在线" : "离线"}
            tone={connected ? "live" : "warn"}
          />
          <StatusTile label="运行时间" value={runtimeLabel} mono />
          <StatusTile
            label="轮询次数"
            value={formatNumber(state.stats.PollCount)}
            mono
          />
          <StatusTile
            label="快照"
            value={formatDateTime(state.generated_at)}
            mono
          />
        </section>

        <div className="grid gap-4 xl:grid-cols-2">
          <SettingSection
            icon={Activity}
            title="运行时"
            rows={[
              { label: "模型", value: valueOrDash(runtime?.model_name), mono: true },
              { label: "Agent", value: valueOrDash(runtime?.agent_type) },
              {
                label: "并发上限",
                value: runtime?.max_concurrency ?? state.stats.MaxAgents,
                mono: true,
              },
              {
                label: "工作区",
                value: valueOrDash(runtime?.workspace_base_dir),
                mono: true,
              },
            ]}
          />

          <SettingSection
            icon={GitBranch}
            title="Tracker"
            rows={[
              { label: "类型", value: valueOrDash(runtime?.tracker_type) },
              {
                label: "范围",
                value: valueOrDash(runtime?.tracker_scope),
                mono: true,
              },
              {
                label: "项目",
                value: valueOrDash(runtime?.project_url),
                mono: true,
              },
            ]}
          />

          <SettingSection
            icon={SlidersHorizontal}
            title="节奏"
            rows={[
              {
                label: "轮询间隔",
                value: formatMilliseconds(runtime?.poll_interval_ms),
                mono: true,
              },
              {
                label: "最大重试退避",
                value: formatMilliseconds(runtime?.max_retry_backoff_ms),
                mono: true,
              },
              {
                label: "Agent 超时",
                value: formatMilliseconds(runtime?.agent_timeout_ms),
                mono: true,
              },
              {
                label: "停滞超时",
                value: formatMilliseconds(runtime?.stall_timeout_ms),
                mono: true,
              },
            ]}
          />

          <SettingSection
            icon={Network}
            title="Hooks 与团队"
            rows={hookRows(runtime).concat(teamRows(runtime))}
          />
        </div>
      </div>
    </div>
  );
}

function hookRows(runtime: RuntimeConfig | undefined): SettingRow[] {
  return [
    {
      label: "before_run",
      value: enabledLabel(runtime?.hooks.before_run),
      mono: true,
    },
    {
      label: "after_run",
      value: enabledLabel(runtime?.hooks.after_run),
      mono: true,
    },
    {
      label: "before_remove",
      value: enabledLabel(runtime?.hooks.before_remove),
      mono: true,
    },
  ];
}

function teamRows(runtime: RuntimeConfig | undefined): SettingRow[] {
  return [
    {
      label: "执行模式",
      value: valueOrDash(runtime?.team.execution_mode),
    },
    {
      label: "Worker",
      value: valueOrDash(runtime?.team.worker_mode),
    },
    {
      label: "Workers",
      value: runtime?.team.max_workers ?? DASH,
      mono: true,
    },
    {
      label: "修复循环",
      value: runtime?.team.max_fix_loops ?? DASH,
      mono: true,
    },
    {
      label: "租约",
      value:
        runtime?.team.claim_lease_seconds !== undefined
          ? `${runtime.team.claim_lease_seconds}s`
          : DASH,
      mono: true,
    },
    {
      label: "状态目录",
      value: valueOrDash(runtime?.team.state_dir),
      mono: true,
    },
    {
      label: "Linear 详情",
      value: runtime ? (runtime.linear.issue_details_enabled ? "开启" : "关闭") : DASH,
    },
    {
      label: "Linear 同步",
      value: !runtime
        ? DASH
        : runtime.linear.sync_comments_enabled
        ? runtime.linear.sync_comments_mode
        : "关闭",
      mono: true,
    },
  ];
}

function SettingSection({
  icon: Icon,
  title,
  rows,
}: {
  icon: ComponentType<{ className?: string }>;
  title: string;
  rows: SettingRow[];
}) {
  return (
    <section className="rounded-lg border border-border/70 bg-card/80 shadow-sm">
      <div className="flex items-center gap-2 border-b border-border/60 px-4 py-3">
        <Icon className="h-4 w-4 text-primary" />
        <h3 className="text-sm font-semibold text-foreground">{title}</h3>
      </div>
      <dl className="divide-y divide-border/60">
        {rows.map((row) => (
          <div
            key={`${title}-${row.label}`}
            className="grid grid-cols-[minmax(7rem,0.6fr)_minmax(0,1.4fr)] items-center gap-3 px-4 py-3"
          >
            <dt className="text-sm text-muted-foreground">{row.label}</dt>
            <dd
              className={`min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-right text-sm text-foreground ${
                row.mono ? "font-mono" : "font-medium"
              }`}
              title={String(row.value)}
            >
              {row.value}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

function StatusTile({
  label,
  value,
  mono,
  tone,
}: {
  label: string;
  value: string | number;
  mono?: boolean;
  tone?: "live" | "warn";
}) {
  return (
    <div className="rounded-lg border border-border/70 bg-card/80 px-4 py-3 shadow-sm">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <div className="mt-2 flex min-w-0 items-center justify-between gap-2">
        <p
          className={`min-w-0 truncate text-lg font-semibold text-foreground ${
            mono ? "font-mono" : ""
          }`}
          title={String(value)}
        >
          {value}
        </p>
        {tone ? (
          <span
            className={`h-2.5 w-2.5 shrink-0 rounded-full ${
              tone === "live" ? "bg-accent" : "bg-destructive"
            }`}
            aria-hidden
          />
        ) : null}
      </div>
    </div>
  );
}

function valueOrDash(value: string | undefined): string {
  const trimmed = value?.trim();
  return trimmed ? trimmed : DASH;
}

function enabledLabel(value: boolean | undefined): string {
  return value ? "已配置" : "未配置";
}

function formatMilliseconds(value: number | undefined): string {
  if (value === undefined || value <= 0) {
    return DASH;
  }
  if (value % 60000 === 0) {
    return `${value / 60000}m`;
  }
  if (value % 1000 === 0) {
    return `${value / 1000}s`;
  }
  return `${value}ms`;
}
