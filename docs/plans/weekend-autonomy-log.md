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

## 2026-05-18 - workspace resolved path helper coverage

- Task selected: add table-driven tests for `resolvedAbs`.
- Why it was valuable: the helper normalizes git worktree paths before registration checks, and GitNexus reported LOW impact with zero affected processes.
- Files changed: `internal/workspace/manager_test.go`.
- Tests run: `go test ./internal/workspace -run TestResolvedAbs -count=1 -v`, `go test ./internal/workspace -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `7f64db3`.
- Remaining follow-up: avoid broader workspace lifecycle edits unless backed by a concrete failing test; those paths are slower and more integration-heavy.

## 2026-05-18 - TUI header helper coverage

- Task selected: add table-driven tests for `projectDetails`, `displayBoardScope`, and `truncateForHeader`.
- Why it was valuable: these helpers shape deterministic header text for tracker scope and board display, and GitNexus reported LOW impact.
- Files changed: `internal/tui/header_test.go`.
- Tests run: `go test ./internal/tui -run "TestProjectDetails|TestDisplayBoardScope|TestTruncateForHeader" -count=1 -v`, `go test ./internal/tui -count=1`, `go test ./... -count=1 -timeout=20m` (failed in existing `internal/agent` timing tests), `go test ./internal/agent -run "TestCodexRunner_HandshakeAndStreamTimeoutsIndependent|TestTimeoutKillsProcess|TestCodexRunner_ConcurrentStartStop|TestCodexRunner_HandshakeTimeout|TestOMXRunner_MissingTeamFailsFast" -count=1 -v`, `go test ./internal/agent -run "TestCodexRunner_HandshakeAndStreamTimeoutsIndependent/handshake_stalls" -count=1 -v`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `4a311ea`.
- Remaining follow-up: investigate `TestCodexRunner_HandshakeAndStreamTimeoutsIndependent/handshake_stalls`; it reproduced independently at about 2.7s against a `<2s` assertion, so the full suite is not green.

## 2026-05-18 - Codex helper timeout test tolerance

- Task selected: relax Codex helper-process timeout ceiling in handshake timeout tests after reproducing the full-suite blocker.
- Why it was valuable: the failing tests were test-only LOW impact, and Windows helper startup/cleanup latency could exceed the old fixed 2s ceiling while still returning the expected handshake timeout.
- Files changed: `internal/agent/codex_test.go`.
- Tests run: `go test ./internal/agent -run "TestCodexRunner_HandshakeAndStreamTimeoutsIndependent/handshake_stalls" -count=1 -v` (red before fix), `go test ./internal/agent -run "TestCodexRunner_HandshakeAndStreamTimeoutsIndependent/handshake_stalls|TestCodexRunner_HandshakeTimeout" -count=1 -v`, `go test ./internal/agent -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `e041c50`.
- Remaining follow-up: keep production Codex lifecycle code untouched unless a separate failing test identifies a runtime bug; this change only adjusts test tolerance.

## 2026-05-18 - GitHub rate-limit reset helper coverage

- Task selected: add table-driven tests for `parseGitHubRateLimitReset`.
- Why it was valuable: GitHub secondary rate-limit handling depends on this parser, and GitNexus reported LOW impact with zero affected processes.
- Files changed: `internal/tracker/github_test.go`.
- Tests run: `go test ./internal/tracker -run TestParseGitHubRateLimitReset -count=1 -v` (red once due non-canonical test header setup, green after using `Header.Set`), `go test ./internal/tracker -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `6637f65`.
- Remaining follow-up: keep direct GitHub network/client behavior changes scoped to concrete failures; this task only pins parser branches.

## 2026-05-18 - Linear issue detail helper coverage

- Task selected: add table-driven tests for Linear issue detail normalization helpers.
- Why it was valuable: these helpers shape rich dashboard metadata from Linear responses, and GitNexus reported LOW impact limited to `FetchIssueDetail`.
- Files changed: `internal/tracker/linear_issue_detail_test.go`.
- Tests run: `go test ./internal/tracker -run "TestLinearUserSummary|TestLinearNamedRef|TestLinearCycleSummary|TestLinearOptionalFloat|TestLinearRelationSummaries" -count=1 -v`, `go test ./internal/tracker -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `a7d412d`.
- Remaining follow-up: continue with pure tracker parser/helper tests after LOW/MEDIUM GitNexus checks; avoid lifecycle and network behavior changes without a concrete failing case.

## 2026-05-18 - dependency parser edge coverage

- Task selected: add dependency parser tests for duplicate refs, markdown checklist lines, and ignoring later non-dependency references.
- Why it was valuable: GitHub issue normalization depends on these parsed dependency IDs, and GitNexus reported LOW impact with zero affected execution flows.
- Files changed: `internal/tracker/deps_test.go`.
- Tests run: `go test ./internal/tracker -run "TestParseDependencies|TestParseBlockedBy" -count=1 -v`, `go test ./internal/tracker -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `4b20100`.
- Remaining follow-up: keep dependency parser changes test-only unless a concrete malformed issue body exposes a production bug.

## 2026-05-18 - TUI team display helper coverage

- Task selected: add table-driven tests for team event, phase, color, and status glyph helpers.
- Why it was valuable: these helpers drive deterministic team table/detail text, and GitNexus reported LOW impact limited to TUI rendering flows.
- Files changed: `internal/tui/detail_view_test.go`, `internal/tui/team_table_test.go`.
- Tests run: `go test ./internal/tui -run "TestCompactTeamEvent|TestCompactTeamPhase|TestTeamPhaseColor|TestTeamStatusGlyph" -count=1 -v`, `go test ./internal/tui -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `ac985a1`.
- Remaining follow-up: consider deterministic `DetailView.RenderTeam` output assertions only if they stay ANSI-normalized and low impact.

## 2026-05-18 - TUI team event data helper coverage

- Task selected: add table-driven tests for terminal team phase detection and team event data conversions.
- Why it was valuable: team events arrive with loosely typed JSON-like data, and GitNexus reported LOW impact through `applyTeamEvent` and `Update`.
- Files changed: `internal/tui/model_test.go`.
- Tests run: `go test ./internal/tui -run "TestIsTerminalTeamPhase|TestStringFromEventData|TestIntFromEventData" -count=1 -v` (first build failed due incorrect test constant names, then passed), `go test ./internal/tui -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`.
- Commit hash: `4b18ed8`.
- Remaining follow-up: keep broader TUI `Update` behavior changes tied to concrete event-flow failures.

## 2026-05-19 - TUI team detail rendering coverage

- Task selected: add deterministic `DetailView.RenderTeam` output assertions.
- Why it was valuable: recent TUI work covered helper-level behavior, but the composed team detail view still lacked stable coverage for the heading, board issue display, worker rows, failed task count, truncation, and event log text.
- Files changed: `internal/tui/detail_view_test.go`.
- Tests run: `go test ./internal/tui -run TestDetailViewRenderTeam -count=1 -v`, `go test ./internal/tui -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `0c6338e`.
- GitNexus impact summary: `DetailView.RenderTeam` was LOW risk with 1 direct caller (`Model.renderDetailContent`), 2 affected TUI processes (`Update`, `syncTables`), and 1 affected module; `compactTeamEvent` was LOW risk with 1 direct caller and 1 affected process.
- Skipped high-risk alternatives: none in this slice.
- Remaining follow-up: continue with deterministic TUI render tests only where assertions can stay ANSI-normalized and avoid spinner or timing state.

## 2026-05-19 - TUI agent detail rendering coverage

- Task selected: add deterministic `DetailView.RenderAgent` output assertions.
- Why it was valuable: agent detail rendering previously had only broad navigation assertions, so this pins the composed heading, stage, PID, age, token summary, turn, session, and event-log detail text without depending on ANSI styling.
- Files changed: `internal/tui/detail_view_test.go`.
- Tests run: `go test ./internal/tui -run TestDetailViewRenderAgent -count=1 -v`, `go test ./internal/tui -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `6e0a0ff`.
- GitNexus impact summary: `DetailView.RenderAgent` was LOW risk with 1 direct caller (`Model.renderDetailContent`), 2 affected TUI processes (`Update`, `syncTables`), and 1 affected module.
- Skipped high-risk alternatives: did not expand `compactEvent` helper coverage in this slice because GitNexus reported HIGH impact through both detail rendering and the agent table.
- Remaining follow-up: consider `Backoff.View` truncation/rune rendering next; keep `compactEvent` changes tied to a concrete failing case.

## 2026-05-19 - TUI backoff truncation coverage

- Task selected: add `Backoff.View` narrow-width coverage and `truncateRunesWithEllipsis` boundary tests.
- Why it was valuable: retry-backoff rows are width-sensitive TUI output, and the helper had no direct coverage for zero/negative limits, short limits, ellipsis behavior, or multi-rune input.
- Files changed: `internal/tui/backoff_test.go`.
- Tests run: `go test ./internal/tui -run "TestBackoffViewOmitsErrorWhenWidthLeavesNoRoom|TestTruncateRunesWithEllipsis" -count=1 -v`, `go test ./internal/tui -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `2d8bec0`.
- GitNexus impact summary: `Backoff.View` was LOW risk with 1 direct caller (`Model.syncTables`), 2 affected TUI processes (`Update`, `syncTables`), and 1 affected module; `truncateRunesWithEllipsis` was LOW risk with 1 direct caller and the same TUI process reach.
- Skipped high-risk alternatives: none in this slice.
- Remaining follow-up: re-rank after three code/test slices; likely next candidates are pure tracker or config helpers unless TUI coverage still has a clearly deterministic gap.

## 2026-05-19 - TUI team bridge nil-input coverage

- Task selected: add table-driven `StartTeamEventBridge` nil-input coverage.
- Why it was valuable: `StartTeamEventBridge` mirrors the agent event bridge but had no direct nil-input coverage, and the test verifies graceful no-panic behavior without timing waits or runtime lifecycle setup.
- Files changed: `internal/tui/model_test.go`.
- Tests run: `go test ./internal/tui -run TestStartTeamEventBridge_NilInputs -count=1 -v`, `go test ./internal/tui -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `d2159c5`.
- GitNexus impact summary: `StartTeamEventBridge` was LOW risk with no direct indexed callers and no affected processes; context still showed `cmd/contrabass/team_root.go` as an incoming file reference.
- Skipped high-risk alternatives: skipped expanding `compactEvent`; GitNexus previously reported HIGH impact through both detail rendering and the agent table.
- Remaining follow-up: selection-boundary helpers are the next viable TUI candidate, but they are lower value than the four completed slices.

## 2026-05-19 - TUI selected detail key bounds

- Task selected: add table-driven bounds tests for `selectedIssueID` and `selectedTeamName`.
- Why it was valuable: detail view routing depends on selected agent/team keys, and the helpers previously lacked direct coverage for wrong-panel, negative-selection, and out-of-range cases.
- Files changed: `internal/tui/navigation_test.go`.
- Tests run: `go test ./internal/tui -run TestModelSelectedDetailKeys -count=1 -v`, `go test ./internal/tui -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `e3f6eb3`.
- GitNexus impact summary: both `selectedIssueID` and `selectedTeamName` were LOW risk, each with 1 direct caller (`Model.renderDetailContent`), 2 affected TUI processes (`Update`, `syncTables`), and 1 affected module.
- Skipped high-risk alternatives: did not expand `compactEvent`; it remains HIGH impact without a concrete failing case.
- Remaining follow-up: table and team-table selected-row helpers are still possible but lower value; stop soon if no stronger candidate appears.

## 2026-05-19 - TUI table selection helper coverage

- Task selected: add table-driven tests for `Table.SelectedRow`, `Table.RowCount`, `TeamTable.SelectedTeam`, `TeamTable.SelectedWorkers`, and `TeamTable.TeamCount`.
- Why it was valuable: these selection helpers are small but feed cursor/detail behavior, and direct coverage now locks valid, negative, out-of-range, and missing-worker-map cases.
- Files changed: `internal/tui/table_test.go`, `internal/tui/team_table_test.go`.
- Tests run: `go test ./internal/tui -run "TestTableSelectedRow|TestTeamTableSelectedTeamAndWorkers" -count=1 -v`, `go test ./internal/tui -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `7463f45`.
- GitNexus impact summary: `SelectedRow`, `RowCount`, `TeamCount`, and `SelectedWorkers` were LOW risk with no indexed upstream impact; `SelectedTeam` was LOW risk with 1 direct caller (`SelectedWorkers`) and no affected processes.
- Skipped high-risk alternatives: continued to avoid `compactEvent` and lifecycle-heavy runtime paths.
- Remaining follow-up: remaining TUI candidates are mostly lower-value coverage around presentation helpers; prefer moving to pure tracker/config helpers only when a clear edge case appears.

## 2026-05-19 - Linear sync mode explicitness coverage

- Task selected: add table-driven tests for `LinearSyncCommentsModeExplicit`.
- Why it was valuable: comment sync behavior needs to distinguish omitted/default modes from explicitly configured modes, including whitespace-only values.
- Files changed: `internal/config/config_test.go`.
- Tests run: `go test ./internal/config -run TestWorkflowConfig_LinearSyncCommentsModeExplicit -count=1 -v`, `go test ./internal/config -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `01bfcdf`.
- GitNexus impact summary: `LinearSyncCommentsModeExplicit` was LOW risk with 1 direct caller (`cmd/contrabass/main.go:run`), 2 affected CLI processes, and 1 affected module.
- Skipped high-risk alternatives: skipped `TeamMaxWorkers`, `TeamMaxFixLoops`, `TeamClaimLeaseSeconds`, and `ValidateWorkerMode` after GitNexus reported HIGH or CRITICAL impact through team execution paths.
- Remaining follow-up: `tracker.IsLinearTracker` is the next viable low-risk helper candidate; avoid config team runtime getters unless a failing test proves a concrete bug.

## 2026-05-19 - Linear tracker detection coverage

- Task selected: add table-driven tests for `tracker.IsLinearTracker` and `LinearClient.IsLinearTracker`.
- Why it was valuable: legacy comment suppression depends on correctly identifying Linear-backed trackers, and the helper previously had no direct nil/non-linear/linear coverage.
- Files changed: `internal/tracker/tracker_test.go`.
- Tests run: `go test ./internal/tracker -run "TestIsLinearTracker|TestLinearClientIsLinearTracker" -count=1 -v`, `go test ./internal/tracker -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `d53a878`.
- GitNexus impact summary: `tracker.IsLinearTracker` was LOW risk with 1 direct orchestrator caller, 1 affected process (`completeRun`), and 1 affected module; `LinearClient.IsLinearTracker` was LOW risk with no indexed upstream impact.
- Skipped high-risk alternatives: continued to avoid config team runtime getters and lifecycle paths.
- Remaining follow-up: inspect only pure helper/error-formatting gaps next; stop if candidates are lifecycle-heavy, redundant, or only coverage-padding.

## 2026-05-19 - Linear error message coverage

- Task selected: add table-driven tests for `RateLimitError.Error` and `AuthError.Error`.
- Why it was valuable: Linear API error handling exposes these messages to callers, and the formatting branches for retry-after and auth status/message had no direct coverage.
- Files changed: `internal/tracker/linear_test.go`.
- Tests run: `go test ./internal/tracker -run TestLinearErrors -count=1 -v`, `go test ./internal/tracker -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `dbb335e`.
- GitNexus impact summary: both Linear error `Error` methods were LOW risk with no indexed upstream impact.
- Skipped high-risk alternatives: continued to avoid config team runtime getters and lifecycle paths.
- Remaining follow-up: remaining tracker gaps are mostly network/lifecycle paths or already-covered parsers; reassess before adding more.

## 2026-05-19 - Workspace mock manager coverage

- Task selected: add tests for `MockManager.List` and `MockManager.CleanupAll`.
- Why it was valuable: workspace mock tests covered create/cleanup/exists but not sorted listing or bulk cleanup, both of which are useful for deterministic in-memory test doubles.
- Files changed: `internal/workspace/mock_test.go`.
- Tests run: `go test ./internal/workspace -run "TestMockManager_ListReturnsSortedIssueIDs|TestMockManager_CleanupAllClearsActiveWorkspaces" -count=1 -v`, `go test ./internal/workspace -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `fe28406`.
- GitNexus impact summary: `MockManager.List` and `MockManager.CleanupAll` were LOW risk with no indexed upstream impact.
- Skipped high-risk alternatives: avoided real workspace lifecycle and git worktree paths.
- Remaining follow-up: remaining workspace gaps are mostly lifecycle or git command paths; stop unless a pure helper remains.

## 2026-05-19 - Local board issue state mapping coverage

- Task selected: add table-driven coverage for `LocalBoardState.IssueState`.
- Why it was valuable: local board state conversion feeds normalized tracker issue state, and the test now locks each board-state mapping plus the unknown-state fallback without touching local tracker lifecycle.
- Files changed: `internal/tracker/local_test.go`.
- Tests run: `go test ./internal/tracker -run TestLocalBoardStateIssueState -count=1 -v`, `go test ./internal/tracker -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `64fe5b8`.
- GitNexus impact summary: `LocalBoardState.IssueState` was LOW risk with no direct non-test callers, no affected processes, and no affected modules.
- Skipped high-risk alternatives: skipped `NewLocalTracker` after GitNexus reported HIGH impact; did not add `BoardDir` coverage because it would be lower-value and constructor-adjacent.
- Remaining follow-up: remaining local tracker candidates are mostly lifecycle/comment paths; stop unless a focused caller-facing behavior gap appears.

## 2026-05-19 - Model override empty-directive fallback coverage

- Task selected: add a focused table case for `ParseModelOverride` when an empty directive appears before a valid model directive.
- Why it was valuable: the parser documentation says it returns the first non-empty model value, and this locks the fallback behavior without changing parser logic.
- Files changed: `internal/tracker/model_override_test.go`.
- Tests run: `go test ./internal/tracker -run TestParseModelOverride -count=1 -v`, `go test ./internal/tracker -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `61a266d`.
- GitNexus impact summary: `ParseModelOverride` was LOW risk with 3 direct callers (`normalizeIssue` for Linear/GitHub and local `toIssue`), 1 affected tracker process, and 1 affected module.
- Skipped high-risk alternatives: continued to avoid local tracker constructor/lifecycle paths and config team getters that previously reported HIGH or CRITICAL impact.
- Remaining follow-up: candidate value is now thin; prefer stopping unless a clearly caller-facing deterministic parser/helper gap is found.

## 2026-05-19 - Web listen address normalization coverage

- Task selected: table-drive `TestNormalizeListenAddr` and add whitespace boundary cases.
- Why it was valuable: the dashboard server listen address normalization is caller-facing startup behavior, and the test now locks defaulting, port-only localhost expansion, and trimming for explicit hosts.
- Files changed: `internal/web/server_test.go`.
- Tests run: `go test ./internal/web -run TestNormalizeListenAddr -count=1 -v`, `go test ./internal/web -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `fee0274`.
- GitNexus impact summary: `normalizeListenAddr` was LOW risk with 1 direct caller (`NewServer`), 2 affected processes (`cmd/contrabass/main.go:run` and `runTeamExecutionWebServer`), and 2 affected modules.
- Skipped high-risk alternatives: did not touch dashboard server lifecycle or HTTP serving paths.
- Remaining follow-up: candidate pool is mostly exhausted; only continue with pure deterministic helpers that expose a real boundary or fallback behavior.

## 2026-05-19 - Update version precedence coverage

- Task selected: add `IsNewer` cases where lower major, minor, or patch versions must not be treated as updates.
- Why it was valuable: update notification behavior depends on semantic version precedence, and the new cases lock comparisons where later components are numerically larger but an earlier component is lower.
- Files changed: `internal/update/update_test.go`.
- Tests run: `go test ./internal/update -run TestIsNewer -count=1 -v`, `go test ./internal/update -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `54e768c`.
- GitNexus impact summary: `IsNewer` was LOW risk with 1 direct caller (`Check`), 2 affected processes (`main` and `Check`), and 2 affected modules.
- Skipped high-risk alternatives: skipped `agent.IsHeartbeatEvent` after GitNexus reported HIGH impact through orchestrator event handling and SSE filtering.
- Remaining follow-up: remaining candidates are increasingly marginal; stop unless the next candidate is a deterministic caller-facing helper with uncovered boundary behavior.

## 2026-05-19 - Team phase helper coverage

- Task selected: add table-driven coverage for `TeamPhase.IsTerminal` and `TeamPhase.ValidTransitions`.
- Why it was valuable: other shared enum helpers already had direct coverage, while team phase terminal and transition behavior feeds team phase governance and previously had no focused helper tests.
- Files changed: `internal/types/types_test.go`.
- Tests run: `go test ./internal/types -run "TestTeamPhaseIsTerminal|TestTeamPhaseValidTransitions" -count=1 -v`, `go test ./internal/types -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `596e421`.
- GitNexus impact summary: `TeamPhase.IsTerminal` and `TeamPhase.ValidTransitions` were LOW risk with no indexed upstream impact, no affected processes, and no affected modules.
- Skipped high-risk alternatives: continued to skip `agent.IsHeartbeatEvent` because GitNexus reported HIGH impact.
- Remaining follow-up: after this three-slice block, re-rank; likely stop unless GitNexus or coverage reveals a focused deterministic gap with LOW/MEDIUM impact.

## 2026-05-19 - Web board not-found helper coverage

- Task selected: add table-driven direct coverage for `isBoardIssueNotFound`.
- Why it was valuable: board handlers map provider not-found errors to 404 responses, and this locks nil, case-insensitive not-found text, and unrelated-error behavior without touching HTTP serving logic.
- Files changed: `internal/web/board_test.go`.
- Tests run: `go test ./internal/web -run TestIsBoardIssueNotFound -count=1 -v`, `go test ./internal/web -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `25a3a98`.
- GitNexus impact summary: `isBoardIssueNotFound` was LOW risk with 2 direct handler callers, 2 affected web processes, and 1 affected module.
- Skipped high-risk alternatives: avoided broader board handler behavior changes and lifecycle paths.
- Remaining follow-up: remaining candidate quality is low; stop after final verification unless a stronger deterministic helper appears.

## 2026-05-19 - Issue detail provider error coverage

- Task selected: add focused handler coverage for issue-detail provider errors.
- Why it was valuable: the endpoint intentionally returns a 502 with the stable snapshot issue when rich provider detail fails, and this caller-facing fallback was not directly tested.
- Files changed: `internal/web/issue_detail_test.go`.
- Tests run: `go test ./internal/web -run TestHandleGetIssueDetails_ProviderError -count=1 -v`, `go test ./internal/web -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `94060d7`.
- GitNexus impact summary: `handleGetIssueDetails` was LOW risk with no indexed upstream impact, no affected processes, and no affected modules.
- Skipped high-risk alternatives: avoided changing issue-detail handler behavior and broader dashboard serving paths.
- Remaining follow-up: remaining web candidates are either already covered or lower-value direct tests around internals; stop unless new failures identify a gap.

## 2026-05-19 - Snapshot issue lookup coverage

- Task selected: add table-driven direct coverage for `Server.issueFromSnapshot`.
- Why it was valuable: issue detail and timeline endpoints rely on this helper to distinguish nil providers, missing issues, and stable snapshot issue lookups.
- Files changed: `internal/web/issue_detail_test.go`.
- Tests run: `go test ./internal/web -run TestIssueFromSnapshot -count=1 -v`, `go test ./internal/web -count=1`, `go test ./... -count=1 -timeout=20m` (first run hit unrelated timeout failures across multiple packages; representative failed tests passed narrowly, and a standard rerun passed), `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `29cccb7`.
- GitNexus impact summary: `issueFromSnapshot` was LOW risk with 2 direct handler callers (`handleGetIssueDetails`, `handleGetIssueTimeline`), 2 affected web processes, and 1 affected module.
- Skipped high-risk alternatives: continued to skip `agent.IsHeartbeatEvent`; avoided broader dashboard serving paths.
- Remaining follow-up: remaining candidates are mostly lower-value direct tests or already-covered behavior; stop soon unless a clearly caller-facing gap remains.

## 2026-05-19 - Invalid board state update coverage

- Task selected: expand board PATCH bad-request coverage to include invalid `state` values.
- Why it was valuable: board clients receive a distinct 400 response for unsupported state values, and the existing bad-request test only covered malformed JSON.
- Files changed: `internal/web/board_test.go`.
- Tests run: `go test ./internal/web -run TestHandleUpdateBoardIssueBadRequest -count=1 -v`, `go test ./internal/web -count=1`, `go test ./... -count=1 -timeout=20m` (first run hit unrelated Codex/orchestrator flakes; the failed tests passed narrowly, and a standard rerun passed), `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `fa031f1`.
- GitNexus impact summary: `handleUpdateBoardIssue` was LOW risk with no indexed upstream impact, no affected processes, and no affected modules.
- Skipped high-risk alternatives: avoided changing board handler behavior and broader board lifecycle paths.
- Remaining follow-up: remaining web candidates are now mostly lower-value direct tests; stop unless a stronger caller-facing gap appears.

## 2026-05-19 - Invalid latest release JSON coverage

- Task selected: add malformed GitHub release JSON coverage for `FetchLatestVersion`.
- Why it was valuable: update checks should surface malformed API responses as fetch errors, and existing tests covered success, HTTP errors, and cancellation but not decode failures.
- Files changed: `internal/update/update_test.go`.
- Tests run: `go test ./internal/update -run TestFetchLatestVersion -count=1 -v`, `go test ./internal/update -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `6e574eb`.
- GitNexus impact summary: `FetchLatestVersion` was LOW risk with 1 direct caller (`Check`), 2 affected processes (`main` and `Check`), and 2 affected modules.
- Skipped high-risk alternatives: avoided broader update-check behavior changes and network-path rewrites.
- Remaining follow-up: after this three-slice block, re-rank; remaining candidates are likely too small unless they cover caller-facing fallback behavior.

## 2026-05-20 - Timeline hidden marker escaping coverage

- Task selected: add table-driven coverage for `HiddenNodeMarker` escaping.
- Why it was valuable: Linear timeline sync idempotency depends on hidden marker attributes staying valid when issue, run, node, or hash values include quotes, backslashes, or double hyphens.
- Files changed: `internal/timeline/store_test.go`.
- Tests run: `go test ./internal/timeline -run TestHiddenNodeMarkerEscapesUnsafeValues -count=1 -v`, `go test ./internal/timeline -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `d1a9706`.
- GitNexus impact summary: `HiddenNodeMarker` was LOW risk with 1 direct caller (`RenderNodeCommentBody`), no affected processes, and 1 affected module (`Timeline`).
- Skipped high-risk alternatives: avoided timeline syncer `Run`/`Drain` lifecycle paths and broader comment sync behavior.
- Remaining follow-up: consider `tracker.IsLinearTracker` or another pure deterministic helper only if it adds caller-facing boundary coverage; remaining candidates are low-value.

## 2026-05-20 - Timeline run marker escaping fix

- Task selected: add a failing test for unsafe values in `RenderRunRootComment` marker attributes, then fix the marker escaping.
- Why it was valuable: run root comments carry hidden idempotency metadata too, and `--` in issue or run IDs leaked into the HTML comment while node markers already escaped that case.
- Files changed: `internal/timeline/render.go`, `internal/timeline/render_test.go`.
- Tests run: `go test ./internal/timeline -run TestRenderRunRootCommentEscapesMarkerValues -count=1 -v` (failed before the fix, passed after), `go test ./internal/timeline -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `99a5459`.
- GitNexus impact summary: `RenderRunRootComment` was LOW impact before editing with 1 direct caller (`LinearSyncer.ensureRoot`), 1 affected process (`Run`), and 1 affected module (`Timeline`); detect after the change reported MEDIUM risk for the same timeline run flow.
- Skipped high-risk alternatives: avoided `LinearSyncer.Run` and `Drain` lifecycle paths; fixed only the proven marker escaping issue.
- Remaining follow-up: re-rank after the next slice; candidate value is now mostly presentation helpers or lifecycle-adjacent paths.

## 2026-05-20 - Timeline node marker escaping fix

- Task selected: add a failing test for unsafe values in `RenderNodeComment` marker attributes, then fix the marker escaping.
- Why it was valuable: `LinearSyncer.syncNode` uses `RenderNodeComment` for live node comments, so the prior `HiddenNodeMarker` coverage did not protect the actual emitted sync marker.
- Files changed: `internal/timeline/render.go`, `internal/timeline/render_test.go`.
- Tests run: `go test ./internal/timeline -run TestRenderNodeCommentEscapesMarkerValues -count=1 -v` (failed before the fix, passed after), `go test ./internal/timeline -count=1`, `go test ./... -count=1 -timeout=20m` (two unrelated flakes reproduced narrowly, third full run passed), `go test ./internal/config -run TestDebounceMultipleRapidEvents -count=1 -v`, `go test ./internal/agent -run TestOMCRunner_UsesTeamRuntime -count=1 -v`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `e94b721`.
- GitNexus impact summary: `RenderNodeComment` was LOW impact before editing with 1 direct caller (`LinearSyncer.syncNode`), 1 affected process (`Run`), and 1 affected module (`Timeline`); detect on this isolated diff did not map changed symbols.
- Skipped high-risk alternatives: avoided node sync lifecycle and retry behavior; changed only marker string rendering after the failing test.
- Remaining follow-up: after three slices, replan; likely stop unless a non-lifecycle deterministic bug is still visible.

## 2026-05-20 - Timeline snapshot boundary record coverage

- Task selected: add table-driven coverage for snapshot loading with unknown records and a final JSONL record without a trailing newline.
- Why it was valuable: timeline snapshots are reduced from append-only JSONL files, and restart/loading behavior should ignore forward-compatible record types while still applying the last valid record when the file has no final newline.
- Files changed: `internal/timeline/store_test.go`.
- Tests run: `go test ./internal/timeline -run TestStoreLoadSnapshotBoundaryRecords -count=1 -v` (first run exposed a bad nil-vs-empty test expectation, corrected and rerun), `go test ./internal/timeline -count=1`, `go test ./... -count=1 -timeout=20m`, `npm run gitnexus:detect` (blocked by multi-repo ambiguity), `npm run gitnexus -- detect-changes --repo contrabass`, `git diff --check`.
- Commit hash: `5a23ddb`.
- GitNexus impact summary: `Store.loadSnapshotNoLock` was LOW impact with 1 direct caller (`LoadSnapshot`), 2 affected processes (`LoadSnapshot`, `Run`), and 2 affected modules (`Timeline`, indirect `Orchestrator`); no production code was edited.
- Skipped high-risk alternatives: avoided changing store locking, append behavior, or syncer lifecycle code.
- Remaining follow-up: remaining candidates are mostly lower-value helper coverage or lifecycle-adjacent syncer tests; prefer stopping after final verification unless a stronger bug appears.
