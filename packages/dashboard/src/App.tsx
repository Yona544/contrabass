import { useEffect, useMemo, useState } from 'react'
import { AppLayout } from './components/AppLayout'
import { useSSE } from './hooks/useSSE'
import { formatDuration } from './i18n/format'
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
  const { state, connected, error } = useSSE()
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

  const runtimeLabel = useMemo(() => formatDuration(runtimeSeconds), [runtimeSeconds])

  if (!state) {
    return (
      <div className="flex h-screen items-center justify-center bg-background text-muted-foreground">
        <p className="text-sm">{zhCN.app.sections.runningSessions}…</p>
      </div>
    )
  }

  return (
    <div className="flex h-screen w-screen flex-col bg-background text-foreground">
      {error ? (
        <div className="border-b border-destructive/40 bg-destructive/10 px-4 py-2 text-xs text-destructive" role="alert">
          {zhCN.app.connectionError}: {error}
        </div>
      ) : null}
      <div className="flex-1 overflow-hidden">
        <AppLayout state={state} connected={connected} runtimeLabel={runtimeLabel} />
      </div>
    </div>
  )
}

export default App
