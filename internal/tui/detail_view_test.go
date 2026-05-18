package tui

import (
	"testing"

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
