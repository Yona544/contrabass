package orchestrator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/history"
	"github.com/junhoyeo/contrabass/internal/tracker"
	"github.com/junhoyeo/contrabass/internal/types"
)

func TestDispatchPausedToggleAndSnapshot(t *testing.T) {
	orch := NewOrchestrator(tracker.NewMockTracker(), nil, nil, nil, nil)

	assert.False(t, orch.DispatchPaused())
	assert.False(t, orch.Snapshot().DispatchPaused)

	orch.SetDispatchPaused(true)
	assert.True(t, orch.DispatchPaused())
	assert.True(t, orch.Snapshot().DispatchPaused)

	orch.SetDispatchPaused(false)
	assert.False(t, orch.DispatchPaused())
}

func TestRetryNowPromotesBackoffEntry(t *testing.T) {
	orch := NewOrchestrator(tracker.NewMockTracker(), nil, nil, nil, nil)
	future := time.Now().Add(time.Hour)
	orch.backoff = []types.BackoffEntry{
		{IssueID: "CB-1", Attempt: 2, RetryAt: future},
		{IssueID: "CB-2", Attempt: 1, RetryAt: future},
	}

	assert.True(t, orch.RetryNow("CB-2"))
	assert.False(t, orch.RetryNow("CB-404"))

	assert.True(t, orch.backoff[1].RetryAt.Before(time.Now().Add(time.Second)))
	assert.Equal(t, future, orch.backoff[0].RetryAt, "other entries untouched")
}

func TestRecordRunHistoryAppendsRecord(t *testing.T) {
	store := history.NewStore(t.TempDir())
	orch := NewOrchestrator(tracker.NewMockTracker(), nil, nil, nil, nil)
	orch.SetRunHistory(store)

	started := time.Now().Add(-90 * time.Second)
	orch.recordRunHistory(
		types.Issue{ID: "CB-9", Identifier: "CB-9", Title: "Ship it"},
		types.RunAttempt{Attempt: 3, Phase: types.Succeeded, TokensIn: 11, TokensOut: 22, StartTime: started},
	)

	records, err := store.Records(0)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "CB-9", records[0].IssueID)
	assert.Equal(t, 3, records[0].Attempt)
	assert.True(t, records[0].Succeeded)
	assert.Equal(t, int64(11), records[0].TokensIn)
	assert.GreaterOrEqual(t, records[0].DurationMs, int64(89_000))
}

func TestRecordRunHistoryNilStoreIsNoop(t *testing.T) {
	orch := NewOrchestrator(tracker.NewMockTracker(), nil, nil, nil, nil)
	orch.recordRunHistory(types.Issue{ID: "CB-1"}, types.RunAttempt{Attempt: 1})
}
