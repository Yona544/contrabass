package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTeamTableBuildRowsTracksFlattenedTeamRows(t *testing.T) {
	tbl := NewTeamTable().Update(
		[]TeamRow{
			{TeamName: "alpha", BoardIssueID: "CB-1", Phase: "team-exec", Workers: 2, ActiveWorkers: 2, Tasks: 4, CompletedTasks: 1, Age: "1m"},
			{TeamName: "beta", Phase: "team-verify", Workers: 1, ActiveWorkers: 1, Tasks: 2, CompletedTasks: 2, Age: "2m"},
		},
		map[string][]TeamWorkerRow{
			"alpha": {
				{WorkerID: "worker-1", Status: "working", CurrentTask: "task-1", PID: 101, Age: "10s"},
				{WorkerID: "worker-2", Status: "working", CurrentTask: "task-2", PID: 102, Age: "11s"},
			},
			"beta": {
				{WorkerID: "worker-3", Status: "idle", CurrentTask: "", PID: 103, Age: "12s"},
			},
		},
		"⠋",
	)

	rows, teamRowIndex := tbl.buildRows()

	assert.Len(t, rows, 5)
	assert.Equal(t, map[int]int{
		0: 0,
		3: 1,
	}, teamRowIndex)
	assert.Equal(t, "alpha · CB-1", rows[0][1])
	assert.Equal(t, "beta", rows[3][1])
	assert.Contains(t, rows[1][1], "├─")
	assert.Contains(t, rows[2][1], "└─")
	assert.Contains(t, rows[4][1], "└─")
}

func TestCompactTeamPhase(t *testing.T) {
	tests := []struct {
		name  string
		phase string
		want  string
	}{
		{name: "plan", phase: "team-plan", want: "Plan"},
		{name: "prd", phase: "team-prd", want: "PRD"},
		{name: "exec", phase: "team-exec", want: "Exec"},
		{name: "verify", phase: "team-verify", want: "Verify"},
		{name: "fix", phase: "team-fix", want: "Fix"},
		{name: "complete", phase: "complete", want: "Done"},
		{name: "failed", phase: "failed", want: "Failed"},
		{name: "cancelled", phase: "cancelled", want: "Cancel"},
		{name: "unknown short", phase: "queued", want: "queued"},
		{name: "unknown long", phase: "team-reviewing", want: "team-rev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, compactTeamPhase(tt.phase))
		})
	}
}

func TestTeamPhaseColor(t *testing.T) {
	tests := []struct {
		name  string
		phase string
		want  string
	}{
		{name: "plan", phase: "team-plan", want: "33"},
		{name: "prd", phase: "team-prd", want: "33"},
		{name: "exec", phase: "team-exec", want: "5"},
		{name: "verify", phase: "team-verify", want: "42"},
		{name: "fix", phase: "team-fix", want: "3"},
		{name: "complete", phase: "complete", want: "42"},
		{name: "failed", phase: "failed", want: "1"},
		{name: "cancelled", phase: "cancelled", want: "1"},
		{name: "unknown", phase: "queued", want: "7"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, teamPhaseColor(tt.phase))
		})
	}
}

func TestTeamStatusGlyph(t *testing.T) {
	tests := []struct {
		name        string
		phase       string
		spinnerView string
		want        string
	}{
		{name: "active phase uses spinner", phase: "team-exec", spinnerView: "spin", want: "spin"},
		{name: "fix phase uses spinner", phase: "team-fix", spinnerView: "spin", want: "spin"},
		{name: "complete phase uses dot", phase: "complete", spinnerView: "spin", want: "●"},
		{name: "failed phase uses dot", phase: "failed", spinnerView: "spin", want: "●"},
		{name: "unknown phase uses dot", phase: "queued", spinnerView: "spin", want: "●"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, teamStatusGlyph(tt.phase, tt.spinnerView))
		})
	}
}
