import { zhCN } from './messages'

const LOCALE = 'zh-CN'

const DASH = '-'

const PHASE_LABELS: Record<number, string> = {
  0: '准备工作区',
  1: '构建提示词',
  2: '启动代理进程',
  3: '初始化会话',
  4: '流式执行',
  5: '收尾',
  6: '成功',
  7: '失败',
  8: '超时',
  9: '停滞',
  10: '已被协调取消',
}

const ISSUE_STATE_LABELS: Record<string, string> = {
  open: '待处理',
  in_progress: '进行中',
  done: '已完成',
}

const TEAM_PHASE_LABELS: Record<string, string> = {
  'team-plan': '规划',
  'team-prd': '需求',
  'team-exec': '执行',
  'team-verify': '验证',
  'team-fix': '修复',
  complete: '完成',
  failed: '失败',
  cancelled: '取消',
}

const WORKER_STATUS_LABELS: Record<string, string> = {
  busy: '忙碌',
  idle: '空闲',
  stopped: '已停止',
}

export function formatDuration(totalSeconds: number): string {
  const safeSeconds = Math.max(0, Math.floor(totalSeconds))
  const hours = Math.floor(safeSeconds / 3600)
  const minutes = Math.floor((safeSeconds % 3600) / 60)
  const seconds = safeSeconds % 60

  if (hours > 0) {
    return `${hours}小时 ${minutes}分 ${seconds}秒`
  }

  return `${minutes}分 ${seconds}秒`
}

export function formatElapsedSince(timestamp: string, nowMs = Date.now()): string {
  const started = Date.parse(timestamp)
  if (Number.isNaN(started)) {
    return DASH
  }

  return formatDuration(Math.floor(Math.max(0, nowMs - started) / 1000))
}

export function formatDateTime(value: string): string {
  const timestamp = Date.parse(value)
  if (Number.isNaN(timestamp)) {
    return DASH
  }

  return new Date(timestamp).toLocaleString(LOCALE, { hour12: false })
}

export function formatTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return zhCN.rateLimits.unknown
  }

  return date.toLocaleTimeString(LOCALE, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
}

export function formatNumber(value: number): string {
  return value.toLocaleString(LOCALE)
}

export function formatCompactNumber(value: number): string {
  if (value >= 100_000_000) {
    return `${trimTrailingZero(value / 100_000_000)}亿`
  }

  if (value >= 10_000) {
    return `${trimTrailingZero(value / 10_000)}万`
  }

  return formatNumber(value)
}

export function formatPhase(phase: number): string {
  return PHASE_LABELS[phase] ?? `${zhCN.rateLimits.unknown}(${phase})`
}

export function formatIssueState(state: string): string {
  return ISSUE_STATE_LABELS[state] ?? ISSUE_STATE_LABELS.open
}

export function formatTeamPhase(phase: string): string {
  return TEAM_PHASE_LABELS[phase] ?? phase
}

export function formatWorkerStatus(status: string): string {
  return WORKER_STATUS_LABELS[status.toLowerCase()] ?? status
}

function trimTrailingZero(value: number): string {
  return value.toFixed(1).replace(/\\.0$/, '')
}
