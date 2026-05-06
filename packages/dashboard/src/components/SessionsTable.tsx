import type { RunningEntry } from '../types'
import { formatElapsedSince, formatNumber, formatPhase } from '../i18n/format'
import { zhCN } from '../i18n/messages'
import './SessionsTable.css'

interface SessionsTableProps {
  entries: RunningEntry[]
}

function formatAge(startedAt: string): string {
  return formatElapsedSince(startedAt)
}

function truncateSessionID(sessionID: string): string {
  return sessionID.slice(0, 8)
}

export function SessionsTable({ entries }: SessionsTableProps) {
  if (entries.length === 0) {
    return <div className="sessions-table__empty">{zhCN.sessions.empty}</div>
  }

  const sortedEntries = [...entries].sort(
    (a, b) => new Date(b.started_at).getTime() - new Date(a.started_at).getTime(),
  )

  return (
    <div className="sessions-table__wrapper">
      <table className="sessions-table" aria-label={zhCN.sessions.ariaLabel}>
        <thead>
          <tr>
            <th>{zhCN.sessions.headers.issueID}</th>
            <th>{zhCN.sessions.headers.phase}</th>
            <th>{zhCN.sessions.headers.pid}</th>
            <th>{zhCN.sessions.headers.age}</th>
            <th>{zhCN.sessions.headers.turns}</th>
            <th>{zhCN.sessions.headers.tokensIn}</th>
            <th>{zhCN.sessions.headers.tokensOut}</th>
            <th>{zhCN.sessions.headers.sessionID}</th>
            <th>{zhCN.sessions.headers.lastEvent}</th>
          </tr>
        </thead>
        <tbody>
          {sortedEntries.map((entry) => (
            <tr key={`${entry.issue_id}-${entry.pid}-${entry.started_at}`}>
              <td>{entry.issue_id}</td>
              <td title={formatPhase(entry.phase)}>{formatPhase(entry.phase)}</td>
              <td className="sessions-table__mono">{entry.pid}</td>
              <td>{formatAge(entry.started_at)}</td>
              <td className="sessions-table__mono">{entry.attempt}</td>
              <td className="sessions-table__mono">{formatNumber(entry.tokens_in)}</td>
              <td className="sessions-table__mono">{formatNumber(entry.tokens_out)}</td>
              <td className="sessions-table__mono" title={entry.session_id}>
                {truncateSessionID(entry.session_id)}
              </td>
              <td className="sessions-table__last-event">-</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export default SessionsTable
