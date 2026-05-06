import './App.css'
import { useEffect, useState } from 'react'
import { Header } from './components/Header'
import { MetricCards } from './components/MetricCards'
import { RateLimits } from './components/RateLimits'
import { QueuePanel } from './components/QueuePanel'
import { RetryQueue } from './components/RetryQueue'
import { SessionsTable } from './components/SessionsTable'
import { TeamTable } from './components/TeamTable'
import { WorkerTable } from './components/WorkerTable'
import { BoardView } from './components/BoardView'
import { AgentLogs } from './components/AgentLogs'
import { useSSE } from './hooks/useSSE'
import { zhCN } from './i18n/messages'

function computeRuntimeSeconds(startTime: string | undefined): number {
  if (!startTime) {
    return 0
  }

  const start = Date.parse(startTime)
  // Guard against Go zero time ("0001-01-01T00:00:00Z") and other pre-epoch dates
  if (Number.isNaN(start) || start <= 0) {
    return 0
  }

  return Math.max(0, Math.floor((Date.now() - start) / 1000))
}

function App() {
  const { state, connected, error, teamSnapshot, boardIssues, agentLogs, queueEvents } = useSSE()
  const [runtimeSeconds, setRuntimeSeconds] = useState(0)
  const startTime = state?.stats.StartTime

  useEffect(() => {
    if (!startTime) {
      setRuntimeSeconds(0)
      return
    }

    setRuntimeSeconds(computeRuntimeSeconds(startTime))

    const timer = window.setInterval(() => {
      setRuntimeSeconds(computeRuntimeSeconds(startTime))
    }, 1000)

    return () => window.clearInterval(timer)
  }, [startTime])

  if (!state) {
    return (
      <div className="dashboard">
        <Header connected={connected} runtimeSeconds={runtimeSeconds} />
        <div className="dashboard__skeleton">
          <div className="dashboard__skeleton-metrics">
            <div className="skeleton-block skeleton-block--card" />
            <div className="skeleton-block skeleton-block--card" />
            <div className="skeleton-block skeleton-block--card" />
          </div>
          <div className="dashboard__skeleton-grid">
            <div className="dashboard__skeleton-primary">
              <div className="skeleton-block skeleton-block--table" />
              <div className="skeleton-block skeleton-block--table" />
            </div>
            <div className="dashboard__skeleton-sidebar">
              <div className="skeleton-block skeleton-block--small" />
              <div className="skeleton-block skeleton-block--small" />
            </div>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="dashboard">
      <Header connected={connected} runtimeSeconds={runtimeSeconds} />

      {error ? (
        <div className="dashboard__notice dashboard__notice--error" role="alert">
          <p className="dashboard__notice-title">{zhCN.app.connectionError}</p>
          <p className="dashboard__notice-message">{error}</p>
        </div>
      ) : null}

      <MetricCards stats={state.stats} backoffCount={(state.backoff ?? []).length} />

      <div className="dashboard__grid">
        <div className="dashboard__primary">
          <section className="dashboard__section">
            <h2 className="dashboard__section-label">{zhCN.app.sections.runningSessions}</h2>
            <SessionsTable entries={state.running ?? []} />
          </section>

          <section className="dashboard__section">
            <h2 className="dashboard__section-label">{zhCN.app.sections.board}</h2>
            <BoardView issues={boardIssues} />
          </section>
        </div>

        <aside className="dashboard__sidebar">
          <section className="dashboard__section">
            <h2 className="dashboard__section-label">Queue</h2>
            <QueuePanel events={queueEvents} />
          </section>

          <section className="dashboard__section">
            <h2 className="dashboard__section-label">{zhCN.app.sections.retryQueue}</h2>
            <RetryQueue entries={state.backoff ?? []} />
          </section>

          <section className="dashboard__section">
            <h2 className="dashboard__section-label">{zhCN.app.sections.rateLimits}</h2>
            <RateLimits limits={[]} />
          </section>

          {teamSnapshot ? (
            <>
              <hr className="dashboard__separator" />
              <section className="dashboard__section">
                <h2 className="dashboard__section-label">{zhCN.app.sections.teamStatus}</h2>
                <TeamTable snapshot={teamSnapshot} />
              </section>

              <section className="dashboard__section">
                <h2 className="dashboard__section-label">{zhCN.app.sections.workers}</h2>
                <WorkerTable workers={teamSnapshot.workers} />
              </section>
            </>
          ) : null}
        </aside>
      </div>

      <section className="dashboard__logs">
        <h2 className="dashboard__section-label">{zhCN.app.sections.agentLogs}</h2>
        <AgentLogs logs={agentLogs} />
      </section>
    </div>
  )
}

export default App
