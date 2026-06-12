# 2026-06-12 — Ten-feature batch

One coordinated batch turning Contrabass into the team's daily driver. Each
feature landed with tests and passed the stable local gate; `gitnexus
detect-changes` was run before every commit (waves through `run`/`createRunner`
rate critical by construction — entry-point wiring — and were covered by full
package suites).

## Landed

| Feature | Where | Notes |
|---------|-------|-------|
| Windows release binaries + Scoop | `.goreleaser.yml`, `release.yml` | windows/amd64+arm64 zips, `scoop-contrabass` bucket; needs `SCOOP_BUCKET_TOKEN` secret and the bucket repo created before the next tag |
| Dashboard auth + `--listen` | `internal/web/auth.go`, `cmd/.../web_options.go` | loopback origins stay open; non-loopback requires token; browser signs in via `/?token=` → HttpOnly cookie (SSE-safe) |
| Jira Cloud tracker | `internal/tracker/jira.go` | JQL polling w/ pagination, dynamic transition resolution by status category, explicit overrides; `JIRA_EMAIL`/`JIRA_API_TOKEN` |
| Webhook notifications | `internal/notify` | Slack-format + generic JSON envelope; default filter = finished/failed/retry/window-closed; non-blocking queue, drops with warn when full |
| Native Claude Code runner | `internal/agent/claudecode.go` | `claude -p --output-format stream-json`, prompt via stdin, codex event taxonomy, stall detection; `agent.type: claude` |
| Auto-PR on verified runs | `internal/orchestrator/pull_request.go` | push + `gh pr create --draft` before workspace cleanup; reuses existing PR (idempotent retries); failures comment, never fail the run |
| Scheduled autonomy | `internal/schedule` | windows/days/budgets; orchestrator gates dispatch per cycle, team mode gates between board dispatches (`ContinueDispatch`); token budget orchestrator-mode only (team layer has no token visibility) |
| Run history + analytics | `internal/history` | append-only JSONL (deliberately not SQLite — repo is file-based and dependency-light; interface allows swapping later); `/api/v1/history`, `/api/v1/analytics` |
| Control plane | `internal/web/control.go`, `internal/orchestrator/control.go` | global pause/resume dispatch (snapshot `dispatch_paused`), promote backoff retry; dashboard UI buttons |
| `init` + `validate` | `cmd/contrabass/init.go`, `validate.go` | scaffold round-trips through the real parser; validate separates hard errors from credential warnings |

## Known follow-ups

- Create the `scoop-contrabass` bucket repo + `SCOOP_BUCKET_TOKEN` secret
  before tagging the next release, or the GoReleaser scoop step fails.
- Team mode does not record run history (orchestrator-mode only); team runs
  already log JSONL events under `.contrabass/state/team/`.
- Notifications in team mode hook `teamRunHooks.EventHandlers`; orchestrator
  mode subscribes to the web event hub.
