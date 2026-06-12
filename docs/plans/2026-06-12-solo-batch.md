# 2026-06-12 — Solo-workflow batch (second ten)

Ten features optimizing contrabass for a solo developer, landed the same day
as the team batch. Same bar: tests per feature, detect-changes per commit
wave, CI green before done.

## Landed

| Feature | Entry point | Notes |
|---------|-------------|-------|
| One-shot runs | `contrabass run "<prompt>"` | ephemeral issue on `.contrabass/oneshot/board`, single-issue tracker filter, no schedule gating, terminal events (released/backoff) end the run; 60s grace fallback |
| Resume with feedback | `contrabass resume ID "fb"` | claude runs continue via `--resume <session>` (history now records model + session id); other agents re-run with feedback appended; liquid `{% raw %}` guards reviewer text |
| Review & land | `contrabass review ID` | branch from `workspace.branch_prefix` + lowercased id; merge `--no-ff` / squash / discard / feedback→resume; refuses dirty tree or wrong base |
| Planner | `contrabass plan "task"` | one claude `-p --output-format json` call → JSON issue array → board issues with BlockedBy wiring; preview by default; injectable executor |
| Plan-approval gate | `approval.require_plan` | first run is plan-only (writes PLAN.md), issue parks (state file per issue under approval.dir), `contrabass approve` / POST /api/v1/issues/{id}/approve unlocks execute run with the plan attached |
| TODO scanner | `contrabass scan` | comment-marker heuristic, `Source: path:line` dedupe, dry-run default |
| Cost analytics | `pricing:` + analytics | claude price table built in (prefix match), cost computed at read time so price edits apply retroactively; `schedule.max_usd` budget; unpriced runs surfaced |
| Prompt recipes | `prompts.dir` | first matching `<label>.md` wins; path-safe label filter |
| Desktop notifications | `notifications.desktop` | powershell.exe WinRT toast (not pwsh — no WinRT projection), osascript, notify-send |
| TUI transcript + editor | `t` / `o` keys | orchestrator emits AgentTranscript events (item/agentMessage text, 4KB cap) |

## Sharp edges worth remembering

- One-shot/resume force `team.execution_mode: single` and clear `schedule:`
  — an explicit run should not wait for a window.
- Plan-only runs skip branch-advance verification and auto-PR (a plan is not
  a code change); the issue returns to Unclaimed but `approvalParked` keeps
  it out of dispatch until approved.
- The approve CLI writes the shared state directory directly — no IPC; a
  running orchestrator picks it up on the next poll.
- Resume reopens the board issue via `UpdateIssueState(Unclaimed)`; the
  single-issue tracker filter prevents the run from picking up anything else.
