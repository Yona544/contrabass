package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCompactTeamEvent(t *testing.T) {
	tests := []struct {
		name  string
		event string
		want  string
	}{
		{name: "team created", event: "team_created", want: "created"},
		{name: "pipeline started", event: "pipeline_started", want: "pipeline start"},
		{name: "pipeline completed", event: "pipeline_completed", want: "pipeline done"},
		{name: "phase started", event: "phase_started", want: "phase start"},
		{name: "task claimed", event: "task_claimed", want: "task claimed"},
		{name: "task completed", event: "task_completed", want: "task done"},
		{name: "task failed", event: "task_failed", want: "task failed"},
		{name: "unknown short", event: "worker_waiting", want: "worker_waiting"},
		{name: "unknown exactly limit", event: "abcdefghijklmnopqrst", want: "abcdefghijklmnopqrst"},
		{name: "unknown long", event: "abcdefghijklmnopqrstu", want: "abcdefghijklmnopq..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, compactTeamEvent(tt.event))
		})
	}
}

func TestDetailViewRenderTeam(t *testing.T) {
	longTask := "abcdefghijklmnopqrstuvwxyz0123456789abcdef"
	rendered := stripANSI(NewDetailView().SetWidth(72).RenderTeam(
		TeamRow{
			TeamName:       "api",
			BoardIssueID:   "CB-42",
			Phase:          "team-fix",
			Workers:        3,
			ActiveWorkers:  1,
			Tasks:          5,
			CompletedTasks: 3,
			FailedTasks:    2,
			FixLoops:       1,
			Age:            "4m",
		},
		[]TeamWorkerRow{
			{WorkerID: "worker-1", Status: "running", CurrentTask: longTask, PID: 4321},
		},
		[]EventLogEntry{
			{
				Timestamp: time.Date(2026, 5, 18, 9, 30, 45, 0, time.UTC),
				Type:      "pipeline_completed",
				Detail:    "verify passed",
			},
		},
	))

	tests := []struct {
		name string
		want string
	}{
		{name: "team heading", want: "TEAM  api"},
		{name: "board issue id", want: "CB-42"},
		{name: "workers section", want: "WORKERS"},
		{name: "failed task count", want: "Tasks: 3/5 (2 failed)"},
		{name: "worker identity", want: "worker-1"},
		{name: "worker status", want: "status: running"},
		{name: "worker truncated task", want: "task: abcdefghijklmnopqrstuvwxyz0123456789a..."},
		{name: "worker pid", want: "pid: 4321"},
		{name: "event log section", want: "EVENT LOG"},
		{name: "compacted team event", want: "09:30:45  pipeline done  verify passed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, rendered, tt.want)
		})
	}

	assert.NotContains(t, rendered, longTask)
	assert.False(t, strings.Contains(rendered, "\x1b["), "rendered output should be ANSI-normalized")
}
