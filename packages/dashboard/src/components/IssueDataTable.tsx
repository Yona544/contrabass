import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { RunningEntry } from '../types'
import { formatElapsedSince, formatNumber, formatRelativeTime } from '../i18n/format'

type LivenessEntry = RunningEntry & {
  phase_label?: string
  last_activity_at?: string
  last_activity_kind?: string
  diff_added?: number
  diff_removed?: number
  diff_files?: number
  diff_status?: string
}

interface IssueDataTableProps {
  entries: LivenessEntry[]
  emptyText: string
  onSelect: (entry: LivenessEntry) => void
  selectedId: string | null
}

function shortId(id: string): string {
  return id.slice(0, 8)
}

function diffSummary(entry: LivenessEntry): string {
  const status = entry.diff_status ?? 'ok'
  if (status !== 'ok') {
    return '?'
  }
  const added = entry.diff_added ?? 0
  const removed = entry.diff_removed ?? 0
  const files = entry.diff_files ?? 0
  if (added === 0 && removed === 0 && files === 0) {
    return '—'
  }
  return `+${added} -${removed} (${files})`
}

function activityTone(at: string | undefined): 'fresh' | 'warm' | 'stale' | 'none' {
  if (!at) return 'none'
  const parsed = Date.parse(at)
  if (Number.isNaN(parsed)) return 'none'
  const ageSeconds = Math.floor(Math.max(0, Date.now() - parsed) / 1000)
  if (ageSeconds < 30) return 'fresh'
  if (ageSeconds <= 180) return 'warm'
  return 'stale'
}

function dotColor(tone: 'fresh' | 'warm' | 'stale' | 'none'): string {
  switch (tone) {
    case 'fresh': return 'bg-emerald-500'
    case 'warm':  return 'bg-amber-500'
    case 'stale': return 'bg-rose-500'
    default:      return 'bg-muted'
  }
}

export function IssueDataTable({ entries, emptyText, onSelect, selectedId }: IssueDataTableProps) {
  if (entries.length === 0) {
    return (
      <div className="flex h-32 items-center justify-center rounded-md border border-dashed text-sm text-muted-foreground">
        {emptyText}
      </div>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="w-24">ID</TableHead>
          <TableHead className="w-32">阶段</TableHead>
          <TableHead className="w-44">上次活动</TableHead>
          <TableHead className="w-32">差异</TableHead>
          <TableHead className="w-20 text-right">已运行</TableHead>
          <TableHead className="w-32 text-right">输入 Token</TableHead>
          <TableHead className="w-32 text-right">输出 Token</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {entries.map((entry) => {
          const tone = activityTone(entry.last_activity_at)
          const isSelected = entry.issue_id === selectedId
          return (
            <TableRow
              key={`${entry.issue_id}-${entry.pid}-${entry.started_at}`}
              data-state={isSelected ? 'selected' : undefined}
              onClick={() => onSelect(entry)}
              className="cursor-pointer"
            >
              <TableCell className="font-mono text-xs">{shortId(entry.issue_id)}</TableCell>
              <TableCell className="text-sm">{entry.phase_label ?? '—'}</TableCell>
              <TableCell className="text-sm">
                <span className="inline-flex items-center gap-2">
                  <span className={`inline-block h-2 w-2 rounded-full ${dotColor(tone)}`} aria-hidden />
                  <span>
                    {entry.last_activity_at ? formatRelativeTime(entry.last_activity_at) : '—'}
                  </span>
                  {entry.last_activity_kind ? (
                    <span className="text-muted-foreground text-xs">{entry.last_activity_kind}</span>
                  ) : null}
                </span>
              </TableCell>
              <TableCell className="font-mono text-xs">{diffSummary(entry)}</TableCell>
              <TableCell className="font-mono text-xs text-right">
                {formatElapsedSince(entry.started_at)}
              </TableCell>
              <TableCell className="font-mono text-xs text-right">
                {formatNumber(entry.tokens_in)}
              </TableCell>
              <TableCell className="font-mono text-xs text-right">
                {formatNumber(entry.tokens_out)}
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
