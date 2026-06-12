import { useEffect, useState } from 'react'
import type { BackoffEntry } from '../types'
import { formatDuration } from '../i18n/format'
import { zhCN } from '../i18n/messages'
import './RetryQueue.css'

interface RetryQueueProps {
  entries: BackoffEntry[]
}

function formatRetryIn(retryAt: string, nowMs: number): { text: string; ready: boolean } {
  const retryAtMs = Date.parse(retryAt)
  if (Number.isNaN(retryAtMs)) {
    return { text: zhCN.retryQueue.unknown, ready: false }
  }

  const diffSeconds = Math.floor((retryAtMs - nowMs) / 1000)
  if (diffSeconds <= 0) {
    return { text: zhCN.retryQueue.ready, ready: true }
  }

  return { text: formatDuration(diffSeconds), ready: false }
}

function truncateError(error: string, limit = 60): string {
  if (error.length <= limit) {
    return error
  }

  return `${error.slice(0, limit - 3)}...`
}

interface RetryActionState {
  status?: 'pending' | 'done'
  error?: string
}

export function RetryQueue({ entries }: RetryQueueProps) {
  const [nowMs, setNowMs] = useState(() => Date.now())
  const [retryState, setRetryState] = useState<Record<string, RetryActionState>>({})

  useEffect(() => {
    const timer = window.setInterval(() => {
      setNowMs(Date.now())
    }, 1000)

    return () => window.clearInterval(timer)
  }, [])

  async function handleRetryNow(issueID: string) {
    setRetryState((current) => ({ ...current, [issueID]: { status: 'pending' } }))
    try {
      const response = await fetch(`/api/v1/backoff/${encodeURIComponent(issueID)}/retry`, {
        method: 'POST',
      })
      if (response.status === 404) {
        setRetryState((current) => ({
          ...current,
          [issueID]: { error: zhCN.retryQueue.retryNotFound },
        }))
        return
      }
      if (!response.ok) {
        const body = (await response.json().catch(() => ({}))) as { error?: string }
        throw new Error(body.error || response.statusText)
      }
      setRetryState((current) => ({ ...current, [issueID]: { status: 'done' } }))
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setRetryState((current) => ({
        ...current,
        [issueID]: { error: zhCN.retryQueue.retryFailed(message) },
      }))
    }
  }

  if (entries.length === 0) {
    return (
      <section className="retry-queue retry-queue--empty" aria-live="polite">
        <p className="retry-queue__empty-text">
          <span className="retry-queue__empty-check" aria-hidden="true">
            ✓
          </span>{' '}
          {zhCN.retryQueue.empty}
        </p>
      </section>
    )
  }

  return (
    <section className="retry-queue" aria-label={zhCN.retryQueue.ariaLabel}>
      <table className="retry-queue__table">
        <thead>
          <tr>
            <th>{zhCN.retryQueue.headers.issueID}</th>
            <th>{zhCN.retryQueue.headers.attempt}</th>
            <th>{zhCN.retryQueue.headers.retryIn}</th>
            <th>{zhCN.retryQueue.headers.error}</th>
            <th>{zhCN.retryQueue.headers.actions}</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((entry) => {
            const retryIn = formatRetryIn(entry.retry_at, nowMs)
            const action = retryState[entry.issue_id] ?? {}
            const actionLocked = action.status === 'pending' || action.status === 'done'

            return (
              <tr key={`${entry.issue_id}-${entry.attempt}-${entry.retry_at}`}>
                <td className="retry-queue__mono">{entry.issue_id}</td>
                <td className="retry-queue__mono">{entry.attempt}</td>
                <td className={`retry-queue__mono ${retryIn.ready ? 'retry-queue__ready' : ''}`}>
                  {retryIn.text}
                </td>
                <td className="retry-queue__error" title={entry.error}>
                  {truncateError(entry.error)}
                </td>
                <td className="retry-queue__actions">
                  <button
                    type="button"
                    className="retry-queue__retry-button"
                    disabled={actionLocked}
                    onClick={() => void handleRetryNow(entry.issue_id)}
                    aria-label={zhCN.retryQueue.retryAria(entry.issue_id)}
                  >
                    {action.status === 'pending'
                      ? zhCN.retryQueue.retrying
                      : action.status === 'done'
                        ? zhCN.retryQueue.retryTriggered
                        : zhCN.retryQueue.retryNow}
                  </button>
                  {action.error ? (
                    <p className="retry-queue__retry-error" role="alert">
                      {action.error}
                    </p>
                  ) : null}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </section>
  )
}
