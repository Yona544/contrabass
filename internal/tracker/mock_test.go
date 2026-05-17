package tracker

import (
	"context"
	"testing"

	"github.com/junhoyeo/contrabass/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockTrackerFetchIssuesReflectsStateUpdates(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		state types.IssueState
	}{
		{name: "claimed", state: types.Claimed},
		{name: "running", state: types.Running},
		{name: "released", state: types.Released},
		{name: "retry queued", state: types.RetryQueued},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockTracker()
			mock.Issues = []types.Issue{{ID: "ISS-1", State: types.Unclaimed}}

			require.NoError(t, mock.UpdateIssueState(ctx, "ISS-1", tt.state))

			issues, err := mock.FetchIssues(ctx)
			require.NoError(t, err)
			require.Len(t, issues, 1)
			assert.Equal(t, tt.state, issues[0].State)
		})
	}
}
