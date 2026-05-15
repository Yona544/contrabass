# Weekend Autonomy Log

## 2026-05-15 - mock tracker state reflection

- Task selected: fix the broad `go test ./...` failure where `tests/e2e TestFullFlow` could miss the second finish under package-level concurrency.
- Why it was valuable: completed mock issues were fetched again as `Unclaimed`, causing redispatch churn and making e2e timing sensitive.
- Files changed: `internal/tracker/mock.go`, `internal/tracker/mock_test.go`.
- Tests run: `go test ./internal/tracker -run TestMockTrackerFetchIssuesReflectsStateUpdates -count=1 -v` (red), `go test ./internal/tracker -run TestMockTrackerFetchIssuesReflectsStateUpdates -count=1`, `go test ./tests/e2e -count=1 -v`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect`.
- Commit hash: `0343134`.
- Remaining follow-up: inspect package coverage for safe test additions now that the baseline is green.

## 2026-05-15 - coverage-mode integration reliability

- Task selected: make `go test ./... -cover` complete on Windows after coverage exposed timing-sensitive integration waits and a transient team state-file rename failure.
- Why it was valuable: coverage instrumentation and package-level concurrency should not make otherwise healthy team/orchestrator/agent tests fail; the rename retry also hardens team state persistence against transient Windows file-sharing races.
- Files changed: `internal/team/store.go`, `internal/agent/codex_test.go`, `internal/agent/tmux_integration_test.go`, `internal/orchestrator/orchestrator_test.go`, `internal/team/goroutine_regression_test.go`, `tests/e2e/team_pipeline_test.go`.
- Tests run: targeted coverage reproductions for agent/orchestrator/team failures, `go test ./internal/team -count=1 -timeout=5m`, `go test ./internal/team -cover -count=1 -timeout=5m`, `go test ./... -cover -count=1 -timeout=20m`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect`.
- Commit hash: `3b4b13b`.
- Remaining follow-up: continue looking for low-risk coverage gaps; GitNexus detect was CRITICAL for this task because the shared team store write helper sits under team execution and recovery flows.
