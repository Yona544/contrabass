import { useMemo, useState } from 'react'
import { SidebarInset, SidebarProvider, SidebarTrigger } from '@/components/ui/sidebar'
import { Separator } from '@/components/ui/separator'
import type { BackoffEntry, Issue, RunningEntry, StateSnapshot } from '../types'
import { AppSidebar, type QueueId } from './AppSidebar'
import { IssueDataTable } from './IssueDataTable'
import { IssueDetailSheet } from './IssueDetailSheet'

interface AppLayoutProps {
  state: StateSnapshot
  connected: boolean
  runtimeLabel: string
}

interface QueueDef {
  id: QueueId
  title: string
  rows: RunningEntry[]
  emptyText: string
}

function getLinearState(issue: Issue): string | undefined {
  const meta = issue.tracker_meta as Record<string, unknown> | undefined
  if (!meta) return undefined
  const v = meta['linear_state']
  return typeof v === 'string' ? v : undefined
}

function backoffAsRunningRow(entry: BackoffEntry, issue?: Issue): RunningEntry {
  return {
    issue_id: entry.issue_id,
    attempt: entry.attempt,
    pid: 0,
    session_id: '',
    workspace: '',
    started_at: entry.retry_at,
    phase: 0,
    tokens_in: 0,
    tokens_out: 0,
    last_activity_at: issue?.updated_at,
    last_activity_kind: 'backoff_enqueued',
    diff_added: 0,
    diff_removed: 0,
    diff_files: 0,
    diff_status: 'ok',
    phase_label: `attempt ${entry.attempt}`,
  } as RunningEntry & { phase_label?: string; last_activity_at?: string; last_activity_kind?: string; diff_added?: number; diff_removed?: number; diff_files?: number; diff_status?: string }
}

function issueAsTodoRow(issue: Issue): RunningEntry {
  return {
    issue_id: issue.id,
    attempt: 0,
    pid: 0,
    session_id: issue.identifier,
    workspace: '',
    started_at: issue.updated_at ?? '',
    phase: 0,
    tokens_in: 0,
    tokens_out: 0,
    phase_label: issue.identifier,
    last_activity_kind: issue.title?.slice(0, 60),
    diff_status: 'ok',
  } as unknown as RunningEntry
}

export function AppLayout({ state, connected, runtimeLabel }: AppLayoutProps) {
  const [active, setActive] = useState<QueueId>('running')
  const [selected, setSelected] = useState<RunningEntry | null>(null)

  const issuesArray = useMemo<Issue[]>(() => Object.values(state.issues ?? {}), [state.issues])

  const queues = useMemo<Record<QueueId, QueueDef>>(() => {
    const running = state.running ?? []
    const backoff = state.backoff ?? []
    const todo = issuesArray.filter((i) => getLinearState(i) === 'Todo')
    const backlog = issuesArray.filter((i) => getLinearState(i) === 'Backlog')
    const done = issuesArray.filter((i) => {
      const s = getLinearState(i)
      return s === 'Done' || s === 'Released'
    })
    const canceled = issuesArray.filter((i) => {
      const s = getLinearState(i)
      return s === 'Canceled' || s === "Won't Do"
    })

    return {
      running: { id: 'running', title: '运行中', rows: running, emptyText: '暂无运行中任务' },
      backoff: { id: 'backoff', title: '退避队列', rows: backoff.map((b) => backoffAsRunningRow(b, state.issues?.[b.issue_id])), emptyText: '退避队列为空' },
      todo: { id: 'todo', title: '待办', rows: todo.map(issueAsTodoRow), emptyText: 'Todo 列表为空' },
      backlog: { id: 'backlog', title: 'Backlog', rows: backlog.map(issueAsTodoRow), emptyText: 'Backlog 列表为空' },
      recent_done: { id: 'recent_done', title: '最近完成', rows: done.slice(0, 50).map(issueAsTodoRow), emptyText: '暂无近期完成任务' },
      canceled: { id: 'canceled', title: '取消', rows: canceled.map(issueAsTodoRow), emptyText: '暂无取消任务' },
    }
  }, [state.running, state.backoff, state.issues, issuesArray])

  const counts: Partial<Record<QueueId, number>> = useMemo(
    () => Object.fromEntries(Object.entries(queues).map(([k, q]) => [k, q.rows.length])) as Partial<Record<QueueId, number>>,
    [queues],
  )

  const currentQueue = queues[active]

  return (
    <SidebarProvider>
      <AppSidebar
        active={active}
        onSelect={(id) => {
          setActive(id)
          setSelected(null)
        }}
        counts={counts}
        connected={connected}
        runtimeLabel={runtimeLabel}
      />
      <SidebarInset>
        <header className="flex h-12 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <h2 className="text-sm font-semibold">{currentQueue.title}</h2>
          <span className="ml-auto text-xs text-muted-foreground">
            {currentQueue.rows.length} 项
          </span>
        </header>
        <div className="flex-1 overflow-auto p-4">
          <IssueDataTable
            entries={currentQueue.rows as Parameters<typeof IssueDataTable>[0]['entries']}
            emptyText={currentQueue.emptyText}
            onSelect={setSelected}
            selectedId={selected?.issue_id ?? null}
          />
        </div>
      </SidebarInset>
      <IssueDetailSheet entry={selected} onClose={() => setSelected(null)} />
    </SidebarProvider>
  )
}
