import { Activity, Clock, Inbox, ListChecks, ListTree, Pause, Play, Settings } from 'lucide-react'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { Badge } from '@/components/ui/badge'

export type QueueId =
  | 'running'
  | 'backoff'
  | 'todo'
  | 'backlog'
  | 'recent_done'
  | 'canceled'

interface AppSidebarProps {
  active: QueueId
  onSelect: (id: QueueId) => void
  counts: Partial<Record<QueueId, number>>
  connected: boolean
  runtimeLabel: string
}

const ACTIVE_GROUP: { id: QueueId; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
  { id: 'running', label: '运行中', icon: Play },
  { id: 'backoff', label: '退避队列', icon: Pause },
]

const QUEUE_GROUP: { id: QueueId; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
  { id: 'todo', label: '待办', icon: ListChecks },
  { id: 'backlog', label: 'Backlog', icon: Inbox },
]

const RECENT_GROUP: { id: QueueId; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
  { id: 'recent_done', label: '最近完成', icon: ListTree },
  { id: 'canceled', label: '取消', icon: Clock },
]

function NavGroup({
  label,
  items,
  active,
  counts,
  onSelect,
}: {
  label: string
  items: typeof ACTIVE_GROUP
  active: QueueId
  counts: Partial<Record<QueueId, number>>
  onSelect: (id: QueueId) => void
}) {
  return (
    <SidebarGroup>
      <SidebarGroupLabel>{label}</SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {items.map((item) => {
            const Icon = item.icon
            const count = counts[item.id]
            return (
              <SidebarMenuItem key={item.id}>
                <SidebarMenuButton
                  isActive={active === item.id}
                  onClick={() => onSelect(item.id)}
                >
                  <Icon className="h-4 w-4" />
                  <span>{item.label}</span>
                </SidebarMenuButton>
                {count !== undefined && count > 0 ? (
                  <SidebarMenuBadge>{count}</SidebarMenuBadge>
                ) : null}
              </SidebarMenuItem>
            )
          })}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  )
}

export function AppSidebar({ active, onSelect, counts, connected, runtimeLabel }: AppSidebarProps) {
  return (
    <Sidebar>
      <SidebarHeader>
        <div className="flex items-center gap-2 px-2 py-1.5">
          <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <Activity className="h-4 w-4" />
          </div>
          <div className="flex min-w-0 flex-col">
            <span className="text-sm font-semibold leading-none">Ziikoo</span>
            <span className="text-xs text-muted-foreground">{runtimeLabel}</span>
          </div>
          <Badge
            variant={connected ? 'default' : 'destructive'}
            className="ml-auto text-[10px]"
          >
            {connected ? '在线' : '离线'}
          </Badge>
        </div>
      </SidebarHeader>
      <SidebarContent>
        <NavGroup label="Active" items={ACTIVE_GROUP} active={active} counts={counts} onSelect={onSelect} />
        <NavGroup label="Queue" items={QUEUE_GROUP} active={active} counts={counts} onSelect={onSelect} />
        <NavGroup label="Recent" items={RECENT_GROUP} active={active} counts={counts} onSelect={onSelect} />
      </SidebarContent>
      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton tooltip="Settings (TODO)">
              <Settings className="h-4 w-4" />
              <span>Settings</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  )
}
