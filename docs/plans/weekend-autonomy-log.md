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

## 2026-05-15 - hooks coverage and timing hardening

- Task selected: add first-party tests for `internal/hooks` and harden two load-sensitive orchestrator waits plus the local dogfood smoke context.
- Why it was valuable: `internal/hooks` had no tests despite running workflow shell commands; the broader suite also showed a few more existing timing assumptions under sustained package concurrency.
- Files changed: `internal/hooks/hooks_test.go`, `cmd/contrabass/dogfood_smoke_test.go`, `internal/orchestrator/orchestrator_test.go`.
- Tests run: `go test ./internal/hooks -count=1 -v`, `go test ./internal/hooks -count=1`, `go test ./internal/hooks -cover -count=1`, `go test ./cmd/contrabass -run TestLocalDogfoodSmokeBoardWorkspaceAndMockAgent -count=1`, `go test ./internal/orchestrator -run 'TestFailedAgentBackoff|TestOrchestrator_FollowUpTurnContinuation' -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect`.
- Commit hash: `5e6a400`.
- Remaining follow-up: use the latest coverage output to pick another low-risk package or exported behavior gap.

## 2026-05-15 - run phase label coverage

- Task selected: add a table-driven test for `RunPhase.Label`.
- Why it was valuable: dashboard-facing phase labels were implemented but not directly covered, leaving enum grouping behavior easy to regress.
- Files changed: `internal/types/types_test.go`.
- Tests run: `go test ./internal/types -run TestRunPhaseLabel -count=1 -v`, `go test ./internal/types -cover -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect`.
- Commit hash: `a158f97`.
- Remaining follow-up: inspect another low-risk package with untested exported behavior before moving to higher-risk reliability code.

## 2026-05-15 - cached update state coverage

- Task selected: add tests for update-check state persistence and cached latest-version behavior.
- Why it was valuable: `internal/update` had no direct coverage for `readState`, `writeState`, or the cached `Check` path even though those helpers influence CLI startup and team-worker update checks.
- Files changed: `internal/update/update_test.go`.
- Tests run: `go test ./internal/update -run 'TestCheckUsesFreshCachedLatest|TestCheckFetchesAndPersistsLatest|TestStateReadWrite|TestReadStateMissingOrInvalid' -count=1 -v`, `go test ./internal/update -coverprofile=$coverPath -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect`.
- Commit hash: `d4b0eea`.
- Remaining follow-up: avoid production edits in `internal/update` without a stronger reason; GitNexus reports `readState` as HIGH impact because CLI and team-worker flows depend on it.

## 2026-05-15 - timeline listing and comment coverage

- Task selected: add tests for timeline issue listing and Linear comment rendering.
- Why it was valuable: `ListIssueIDs` and the render helpers were low-risk coverage gaps in a package that persists workflow progress and publishes Linear comments.
- Files changed: `internal/timeline/store_test.go`, `internal/timeline/render_test.go`.
- Tests run: `go test ./internal/timeline -run 'TestStoreListIssueIDs|TestRenderRunRootComment|TestRenderNodeComment' -count=1 -v`, `go test ./internal/timeline -coverprofile=$timelineCover -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect`.
- Commit hash: `686c5fb`.
- Remaining follow-up: leave `RunID`, `NodeID`, `Drain`, and `Run` production behavior untouched without a concrete need; GitNexus reports those paths as HIGH impact.

## 2026-05-15 - tmux lifecycle helper coverage

- Task selected: add tests for tmux session create-if-missing, session kill, and shell quoting behavior.
- Why it was valuable: these low-risk helpers build tmux commands used by the agent runtime, and the tests pin down command ordering, validation errors, and quote escaping.
- Files changed: `internal/tmux/session_test.go`, `internal/tmux/bootstrap_test.go`.
- Tests run: `go test ./internal/tmux -run 'TestSessionCreateIfNotExists|TestSessionKill|TestShellQuote' -count=1 -v`, `go test ./internal/tmux -coverprofile $p -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect`.
- Commit hash: `a34834a`.
- Remaining follow-up: consider CLI registry validation tests next; avoid touching actual tmux command execution without a concrete runtime failure.

## 2026-05-15 - tmux CLI registry validation coverage

- Task selected: add tests for CLI registry validation, prompt-mode checks, nil/empty list handling, and `mustRegister` panic behavior.
- Why it was valuable: registry validation protects tmux-backed agent launcher configuration, and this completed coverage of the branchy validation helpers without changing runtime code.
- Files changed: `internal/tmux/cli_registry_test.go`.
- Tests run: `go test ./internal/tmux -run 'TestCLIRegistry_RegisterValidationErrors|TestCLIRegistry_RegisterTrimsAgentType|TestCLIRegistry_ListEmptyForNilOrEmptyRegistry|TestMustRegisterPanicsOnInvalidConfig|TestIsValidPromptMode' -count=1 -v`, `go test ./internal/tmux -coverprofile $p -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect`.
- Commit hash: `eacbcd2`.
- Remaining follow-up: move to broader packages only with focused targets; remaining tmux gaps are mostly command-execution and lifecycle integration paths.

## 2026-05-17 - board assign CLI coverage

- Task selected: add a focused CLI test for `board assign`.
- Why it was valuable: `runBoardAssign` was only lightly covered despite persisting assignees used by board dispatch and team routing.
- Files changed: `cmd/contrabass/board_test.go`.
- Tests run: `go test ./cmd/contrabass -run TestBoardAssignCommandUpdatesAssignee -count=1 -v`, `go test ./cmd/contrabass -coverprofile $p -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `2cdec2a`.
- Remaining follow-up: continue with narrow CLI tests around low-coverage helper paths before moving into `internal/agent`.

## 2026-05-17 - doctor runtime tool selection coverage

- Task selected: add table-driven tests for `agentRuntimeTool`.
- Why it was valuable: doctor runtime readiness depends on selecting the right executable and remediation message for each supported agent type.
- Files changed: `cmd/contrabass/doctor_test.go`.
- Tests run: `go test ./cmd/contrabass -run TestAgentRuntimeTool -count=1 -v`, `go test ./cmd/contrabass -coverprofile $p -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `75c9d38`.
- Remaining follow-up: skip `binaryPathForAgent` for now unless there is a concrete failure; GitNexus reports it as CRITICAL because team execution and worker startup depend on it.

## 2026-05-17 - team CLI error formatting coverage

- Task selected: add a focused test for `teamCLIError.Error`.
- Why it was valuable: this is a pure agent helper used to surface team CLI failures, and it improved `internal/agent` coverage without entering process lifecycle code.
- Files changed: `internal/agent/teamcli_test.go`.
- Tests run: `go test ./internal/agent -run TestTeamCLIErrorError -count=1 -v`, `go test ./internal/agent -coverprofile $p -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `c715e4a`.
- Remaining follow-up: avoid `IsHeartbeatEvent` unless needed; GitNexus reports it as HIGH because orchestrator and SSE filtering depend on it.

## 2026-05-17 - Codex overload helper coverage

- Task selected: add tests for Codex overload retry delays and JSON-RPC overload error detection.
- Why it was valuable: overload handling for JSON-RPC error `-32001` is a project target area, and these pure helpers can be covered without starting an agent process.
- Files changed: `internal/agent/codex_test.go`.
- Tests run: `go test ./internal/agent -run 'TestCodexRunner_OverloadRetryDelay|TestIsCodexOverloadRPCError' -count=1 -v`, `go test ./internal/agent -coverprofile $p -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `36eb30f`.
- Remaining follow-up: future agent work should keep the same narrow shape; process lifecycle paths are slower and higher blast radius.

## 2026-05-17 - doctor path normalization coverage

- Task selected: add tests for `doctorAbsPath`.
- Why it was valuable: doctor diagnostics rely on normalized paths for repo/config/writable checks, and this covers blank, relative, and absolute inputs without touching production code.
- Files changed: `cmd/contrabass/doctor_test.go`.
- Tests run: `go test ./cmd/contrabass -run TestDoctorAbsPath -count=1 -v`, `go test ./cmd/contrabass -coverprofile $p -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `7341dd3`.
- Remaining follow-up: continue with CLI helper tests only where GitNexus impact is LOW; avoid broad team runtime execution paths without a failing case.

## 2026-05-17 - CLI helper coverage

- Task selected: add focused tests for `firstCommandToken`, `appendUniqueString`, and `stringFromMap`.
- Why it was valuable: these low-impact helpers shape doctor runtime diagnostics and board sync metadata; the `firstCommandToken` test exposed and fixed quoted executable paths containing spaces.
- Files changed: `cmd/contrabass/doctor.go`, `cmd/contrabass/doctor_test.go`, `cmd/contrabass/team_board_test.go`.
- Tests run: `go test ./cmd/contrabass -run 'TestFirstCommandToken|TestAppendUniqueString|TestStringFromMap' -count=1 -v`, `go test ./cmd/contrabass -coverprofile $p -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `c6653f3`.
- Remaining follow-up: continue checking helper candidates case by case; skip `formatCommandOutput` unless a concrete bug justifies the HIGH impact path.

## 2026-05-17 - writable path and child retry helper coverage

- Task selected: add tests for doctor writable-path probing and board child retry marking.
- Why it was valuable: doctor readiness depends on probing existing parents without creating target directories, and board finalization must requeue only claimed child issues when a team run aborts.
- Files changed: `cmd/contrabass/doctor_test.go`, `cmd/contrabass/team_board_test.go`.
- Tests run: `go test ./cmd/contrabass -run 'TestNearestExistingDir|TestCheckWritablePath|TestBoardIssueSyncerMarkClaimedChildIssuesForRetry' -count=1 -v`, `go test ./cmd/contrabass -coverprofile $p -count=1`, `go test ./cmd/contrabass -run TestRun_DefaultInternalWorkflowUsesTeamExecution -count=1 -v`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `319bc99`.
- Remaining follow-up: continue avoiding `cloneStringMap` and team execution changes without a concrete bug; GitNexus reports that path as HIGH impact.

## 2026-05-17 - narrow agent helper coverage

- Task selected: add focused tests for Codex nested string extraction, OpenCode event payload helpers, team task seed selection, phase summarization, primary task selection, and JSON map conversion.
- Why it was valuable: these helpers shape agent event parsing and team-monitor summaries, and they can be verified without starting external agent processes.
- Files changed: `internal/agent/codex_test.go`, `internal/agent/opencode_test.go`, `internal/agent/teamcli_test.go`.
- Tests run: `go test ./internal/agent -run 'TestBuildTeamTaskSeed|TestSummarizePhase|TestPrimaryTask|TestMustJSONMap|TestOpenCodeEventPayloadSessionID|TestOpenCodeEventPayloadIdle|TestExtractListeningURL|TestExtractNestedString' -count=1 -v`, `go test ./internal/agent -coverprofile $p -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `e3761f1`.
- Remaining follow-up: keep `firstNonEmpty`, `formatCommandOutput`, and lifecycle paths untouched without a concrete bug; GitNexus reports those paths as HIGH impact or integration-heavy.

## 2026-05-18 - local team matching helper coverage

- Task selected: add table-driven tests for `normalizeLocalTeamName` and `issueMatchesTeam`.
- Why it was valuable: these helpers control local board team dispatch matching, and GitNexus reported LOW impact while related lifecycle tests only covered the behavior indirectly.
- Files changed: `internal/tracker/local_test.go`.
- Tests run: `go test ./internal/tracker -run "TestNormalizeLocalTeamName|TestIssueMatchesTeam" -count=1 -v`, `go test ./internal/tracker -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `a8a0ce3`.
- Remaining follow-up: skip `sanitizeLocalIssuePrefix` without a concrete bug; GitNexus reported HIGH impact because it feeds `NewLocalTracker` and board/team startup flows.
