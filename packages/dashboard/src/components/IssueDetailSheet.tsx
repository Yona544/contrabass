import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import type { RunningEntry } from '../types'

interface IssueDetailSheetProps {
  entry: RunningEntry | null
  onClose: () => void
}

/**
 * Empty shell from N1 — full Tier 1/2/3 layout is implemented in N3
 * (ZII-60).  For now we just show the issue id + a note so users see
 * the shell open/close behavior is wired correctly.
 */
export function IssueDetailSheet({ entry, onClose }: IssueDetailSheetProps) {
  const open = entry !== null

  return (
    <Sheet open={open} onOpenChange={(next) => { if (!next) onClose() }}>
      <SheetContent side="right" className="sm:max-w-[480px]">
        <SheetHeader>
          <SheetTitle className="font-mono text-sm">
            {entry ? entry.issue_id.slice(0, 8) + '…' : ''}
          </SheetTitle>
          <SheetDescription>
            详情面板（占位） — 完整 Tier 1/2/3 渲染由 ZII-60 (N3) 实现。
          </SheetDescription>
        </SheetHeader>

        {entry ? (
          <div className="px-6 py-4 space-y-3 text-sm">
            <div className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5">
              <span className="text-muted-foreground">PID</span>
              <span className="font-mono">{entry.pid}</span>

              <span className="text-muted-foreground">attempt</span>
              <span className="font-mono">{entry.attempt}</span>

              <span className="text-muted-foreground">tokens</span>
              <span className="font-mono">
                {entry.tokens_in.toLocaleString()} / {entry.tokens_out.toLocaleString()}
              </span>

              <span className="text-muted-foreground">workspace</span>
              <span className="font-mono break-all text-xs">{entry.workspace}</span>

              <span className="text-muted-foreground">session</span>
              <span className="font-mono break-all text-xs">{entry.session_id}</span>
            </div>
          </div>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}
