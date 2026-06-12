package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func record(issueID, agent string, succeeded bool, tokensIn, tokensOut, durationMs int64, finished time.Time) Record {
	return Record{
		IssueID:    issueID,
		AgentType:  agent,
		Attempt:    1,
		Phase:      "Succeeded",
		Succeeded:  succeeded,
		TokensIn:   tokensIn,
		TokensOut:  tokensOut,
		StartedAt:  finished.Add(-time.Duration(durationMs) * time.Millisecond),
		FinishedAt: finished,
		DurationMs: durationMs,
	}
}

func TestStoreAppendAndRecordsNewestFirst(t *testing.T) {
	store := NewStore(t.TempDir())
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	require.NoError(t, store.Append(record("CB-1", "claude", true, 100, 50, 1000, base)))
	require.NoError(t, store.Append(record("CB-2", "codex", false, 200, 80, 2000, base.Add(time.Hour))))
	require.NoError(t, store.Append(record("CB-3", "claude", true, 300, 90, 3000, base.Add(2*time.Hour))))

	records, err := store.Records(0)
	require.NoError(t, err)
	require.Len(t, records, 3)
	assert.Equal(t, "CB-3", records[0].IssueID)
	assert.Equal(t, "CB-1", records[2].IssueID)

	limited, err := store.Records(2)
	require.NoError(t, err)
	require.Len(t, limited, 2)
	assert.Equal(t, "CB-3", limited[0].IssueID)
}

func TestStoreRecordsEmptyWithoutFile(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "missing"))
	records, err := store.Records(10)
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestStoreSkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, store.Append(record("CB-1", "claude", true, 1, 1, 1, base)))

	f, err := os.OpenFile(filepath.Join(dir, runsFileName), os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("{torn write\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, store.Append(record("CB-2", "claude", true, 1, 1, 1, base.Add(time.Hour))))

	records, err := store.Records(0)
	require.NoError(t, err)
	assert.Len(t, records, 2)
}

func TestAnalyticsAggregatesByAgent(t *testing.T) {
	store := NewStore(t.TempDir())
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	require.NoError(t, store.Append(record("CB-1", "claude", true, 100, 50, 1000, base)))
	require.NoError(t, store.Append(record("CB-2", "claude", false, 200, 60, 3000, base.Add(time.Hour))))
	require.NoError(t, store.Append(record("CB-3", "codex", true, 400, 70, 2000, base.Add(2*time.Hour))))

	analytics, err := store.Analytics()
	require.NoError(t, err)

	assert.Equal(t, 3, analytics.TotalRuns)
	assert.Equal(t, 2, analytics.Succeeded)
	assert.Equal(t, 1, analytics.Failed)
	assert.Equal(t, int64(700), analytics.TokensIn)
	assert.Equal(t, int64(180), analytics.TokensOut)
	assert.Equal(t, int64(2000), analytics.AvgDurationMs)

	claude := analytics.ByAgent["claude"]
	assert.Equal(t, 2, claude.Runs)
	assert.Equal(t, 1, claude.Succeeded)
	assert.Equal(t, int64(2000), claude.AvgDurationMs)

	codex := analytics.ByAgent["codex"]
	assert.Equal(t, 1, codex.Runs)
	assert.Equal(t, 0, codex.Failed)
}

func TestAnalyticsEmptyHistory(t *testing.T) {
	store := NewStore(t.TempDir())
	analytics, err := store.Analytics()
	require.NoError(t, err)
	assert.Equal(t, 0, analytics.TotalRuns)
	assert.NotNil(t, analytics.ByAgent)
}
