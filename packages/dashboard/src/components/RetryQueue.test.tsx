import { afterEach, describe, expect, it, mock } from 'bun:test'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import type { BackoffEntry } from '../types'
import { RetryQueue } from './RetryQueue'
import { zhCN } from '../i18n/messages'

function expectInDocument(value: unknown) {
  ;(expect(value) as any).toBeInTheDocument()
}

function jsonResponse(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function installFetchMock(
  handler: (url: string, init?: RequestInit) => Response | Promise<Response>,
) {
  const original = globalThis.fetch
  const fetchMock = mock((input: RequestInfo | URL, init?: RequestInit) =>
    handler(String(input), init),
  )
  globalThis.fetch = fetchMock as unknown as typeof fetch
  return {
    fetchMock,
    restore: () => {
      globalThis.fetch = original
    },
  }
}

afterEach(() => {
  cleanup()
  mock.restore()
})

describe('RetryQueue', () => {
  it('renders retry rows and error messages', () => {
    const longError = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890'
    const entries: BackoffEntry[] = [
      {
        issue_id: 'ISSUE-10',
        attempt: 1,
        retry_at: 'not-a-date',
        error: 'failed to acquire lock',
      },
      {
        issue_id: 'ISSUE-20',
        attempt: 2,
        retry_at: '2000-01-01T00:00:00.000Z',
        error: 'process timed out',
      },
      {
        issue_id: 'ISSUE-30',
        attempt: 3,
        retry_at: '2000-01-01T00:00:00.000Z',
        error: longError,
      },
    ]

    render(<RetryQueue entries={entries} />)

    expectInDocument(screen.getByRole('table'))
    expectInDocument(screen.getByText('ISSUE-10'))
    expectInDocument(screen.getByText('ISSUE-20'))
    expectInDocument(screen.getByText('failed to acquire lock'))
    expectInDocument(screen.getByText('process timed out'))
    expectInDocument(screen.getByText(`${longError.slice(0, 57)}...`))
    expectInDocument(screen.getByText('未知'))
    expect(screen.getAllByText('就绪')).toHaveLength(2)
  })

  it('renders empty state when queue is empty', () => {
    render(<RetryQueue entries={[]} />)

    expectInDocument(screen.getByText('暂无待重试任务'))
  })
})

describe('RetryQueue — retry now action', () => {
  const entry: BackoffEntry = {
    issue_id: 'ISSUE-10',
    attempt: 2,
    retry_at: '2000-01-01T00:00:00.000Z',
    error: 'process timed out',
  }

  it('posts to the retry endpoint and disables the button while pending', async () => {
    let resolveRetry!: () => void
    const retryPromise = new Promise<Response>((resolve) => {
      resolveRetry = () => resolve(new Response(null, { status: 202 }))
    })

    const calls: Array<{ url: string; method?: string }> = []
    const { restore } = installFetchMock((url, init) => {
      calls.push({ url, method: init?.method })
      return retryPromise
    })

    try {
      render(<RetryQueue entries={[entry]} />)

      fireEvent.click(
        screen.getByRole('button', { name: zhCN.retryQueue.retryAria('ISSUE-10') }),
      )

      await waitFor(() => {
        ;(expect(screen.getByText(zhCN.retryQueue.retrying)) as any).toBeDisabled()
      })

      expect(calls).toEqual([
        { url: '/api/v1/backoff/ISSUE-10/retry', method: 'POST' },
      ])

      resolveRetry()

      await waitFor(() => {
        ;(expect(
          screen.getByText(zhCN.retryQueue.retryTriggered),
        ) as any).toBeDisabled()
      })
    } finally {
      restore()
    }
  })

  it('surfaces 404 (not in retry queue) non-fatally and re-enables the button', async () => {
    const { restore } = installFetchMock(() =>
      jsonResponse({ error: 'issue not in retry queue' }, 404),
    )

    try {
      render(<RetryQueue entries={[entry]} />)

      fireEvent.click(
        screen.getByRole('button', { name: zhCN.retryQueue.retryAria('ISSUE-10') }),
      )

      await waitFor(() => {
        expectInDocument(screen.getByText(zhCN.retryQueue.retryNotFound))
      })

      // Row is still rendered and the button is usable again.
      expectInDocument(screen.getByText('ISSUE-10'))
      ;(expect(
        screen.getByRole('button', { name: zhCN.retryQueue.retryAria('ISSUE-10') }),
      ) as any).not.toBeDisabled()
      expectInDocument(screen.getByText(zhCN.retryQueue.retryNow))
    } finally {
      restore()
    }
  })

  it('surfaces other failures with an inline error message', async () => {
    const { restore } = installFetchMock(() =>
      jsonResponse({ error: 'backoff manager offline' }, 500),
    )

    try {
      render(<RetryQueue entries={[entry]} />)

      fireEvent.click(
        screen.getByRole('button', { name: zhCN.retryQueue.retryAria('ISSUE-10') }),
      )

      await waitFor(() => {
        expectInDocument(
          screen.getByText(zhCN.retryQueue.retryFailed('backoff manager offline')),
        )
      })
    } finally {
      restore()
    }
  })
})
