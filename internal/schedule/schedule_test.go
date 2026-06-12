package schedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clock builds a time on the given weekday at HH:MM. June 2026: the 6th is a
// Saturday.
func clock(day time.Weekday, hour, minute int) time.Time {
	base := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC) // Saturday
	offset := (int(day) - int(time.Saturday) + 7) % 7
	return base.AddDate(0, 0, offset).Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
}

func TestNewRejectsBadInput(t *testing.T) {
	cases := []Config{
		{Windows: []string{"22:00"}},
		{Windows: []string{"25:00-26:00"}},
		{Windows: []string{"10:00-10:00"}},
		{Days: []string{"caturday"}},
	}
	for _, cfg := range cases {
		_, err := New(cfg)
		assert.Error(t, err, "config %+v", cfg)
	}
}

func TestActiveAtDayFilterWithoutWindows(t *testing.T) {
	s, err := New(Config{Days: []string{"sat", "sun"}})
	require.NoError(t, err)

	assert.True(t, s.activeAt(clock(time.Saturday, 13, 0)))
	assert.True(t, s.activeAt(clock(time.Sunday, 3, 0)))
	assert.False(t, s.activeAt(clock(time.Monday, 13, 0)))
}

func TestActiveAtSimpleWindow(t *testing.T) {
	s, err := New(Config{Windows: []string{"09:00-17:00"}})
	require.NoError(t, err)

	assert.False(t, s.activeAt(clock(time.Monday, 8, 59)))
	assert.True(t, s.activeAt(clock(time.Monday, 9, 0)))
	assert.True(t, s.activeAt(clock(time.Monday, 16, 59)))
	assert.False(t, s.activeAt(clock(time.Monday, 17, 0)))
}

func TestActiveAtOvernightWindowSpillsIntoNextDay(t *testing.T) {
	s, err := New(Config{Windows: []string{"22:00-06:00"}, Days: []string{"fri"}})
	require.NoError(t, err)

	assert.True(t, s.activeAt(clock(time.Friday, 23, 0)), "friday evening")
	assert.True(t, s.activeAt(clock(time.Saturday, 5, 0)), "saturday early morning belongs to friday window")
	assert.False(t, s.activeAt(clock(time.Saturday, 7, 0)))
	assert.False(t, s.activeAt(clock(time.Thursday, 23, 0)))
}

func TestAllowDispatchOutsideWindow(t *testing.T) {
	s, err := New(Config{Windows: []string{"09:00-10:00"}})
	require.NoError(t, err)

	ok, reason := s.AllowDispatch(clock(time.Monday, 11, 0))
	assert.False(t, ok)
	assert.Equal(t, "outside schedule window", reason)
}

func TestAllowDispatchIssueBudget(t *testing.T) {
	s, err := New(Config{MaxIssues: 2})
	require.NoError(t, err)
	now := clock(time.Monday, 11, 0)

	ok, _ := s.AllowDispatch(now)
	require.True(t, ok)

	s.RecordStart(now)
	s.RecordStart(now)

	ok, reason := s.AllowDispatch(now)
	assert.False(t, ok)
	assert.Contains(t, reason, "issue budget exhausted (2/2)")
}

func TestAllowDispatchTokenBudget(t *testing.T) {
	s, err := New(Config{MaxTokens: 1000})
	require.NoError(t, err)
	now := clock(time.Monday, 11, 0)

	s.RecordCompletion(true, 600, 0)
	ok, _ := s.AllowDispatch(now)
	require.True(t, ok, "under budget still dispatches")

	s.RecordCompletion(false, 600, 0)
	ok, reason := s.AllowDispatch(now)
	assert.False(t, ok)
	assert.Contains(t, reason, "token budget exhausted (1200/1000)")
}

func TestAllowDispatchCostBudget(t *testing.T) {
	s, err := New(Config{MaxUSD: 5})
	require.NoError(t, err)
	now := clock(time.Monday, 11, 0)

	s.RecordCompletion(true, 100, 3.0)
	ok, _ := s.AllowDispatch(now)
	require.True(t, ok)

	s.RecordCompletion(true, 100, 2.5)
	ok, reason := s.AllowDispatch(now)
	assert.False(t, ok)
	assert.Contains(t, reason, "cost budget exhausted ($5.50/$5.00)")
}

func TestTickWindowLifecycleAndSummary(t *testing.T) {
	s, err := New(Config{Windows: []string{"09:00-10:00"}, MaxIssues: 5})
	require.NoError(t, err)

	// Window opens.
	summary, closed := s.Tick(clock(time.Monday, 9, 0))
	assert.False(t, closed)
	assert.Empty(t, summary)

	s.RecordStart(clock(time.Monday, 9, 5))
	s.RecordCompletion(true, 200, 0)
	s.RecordStart(clock(time.Monday, 9, 20))
	s.RecordCompletion(false, 300, 0)

	// Still inside the window: no summary.
	_, closed = s.Tick(clock(time.Monday, 9, 30))
	assert.False(t, closed)

	// Window closes with a summary of what happened.
	summary, closed = s.Tick(clock(time.Monday, 10, 1))
	require.True(t, closed)
	assert.Contains(t, summary, "2 runs started")
	assert.Contains(t, summary, "1 succeeded")
	assert.Contains(t, summary, "1 failed")
	assert.Contains(t, summary, "500 tokens used")

	// Next window opening resets the budget counters.
	_, closed = s.Tick(clock(time.Tuesday, 9, 0))
	assert.False(t, closed)
	ok, _ := s.AllowDispatch(clock(time.Tuesday, 9, 1))
	assert.True(t, ok)
}
