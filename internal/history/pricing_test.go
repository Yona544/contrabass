package history

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostUsesDefaultPrefixPricing(t *testing.T) {
	store := NewStore(t.TempDir())

	// claude-sonnet-4-6 matches the claude-sonnet prefix: $3/$15 per MTok.
	cost := store.Cost("claude-sonnet-4-6", 1_000_000, 1_000_000)
	assert.InDelta(t, 18.0, cost, 1e-9)

	// Unknown models are unpriced.
	assert.Zero(t, store.Cost("mystery-model", 1_000_000, 1_000_000))
	assert.Zero(t, store.Cost("", 1_000_000, 1_000_000))
}

func TestSetPricingOverridesAndExtends(t *testing.T) {
	store := NewStore(t.TempDir())
	store.SetPricing(map[string]ModelPrice{
		"claude-sonnet": {InputPerMTok: 1, OutputPerMTok: 2},
		"codex-mini":    {InputPerMTok: 0.5, OutputPerMTok: 2.5},
	})

	assert.InDelta(t, 3.0, store.Cost("claude-sonnet-4-6", 1_000_000, 1_000_000), 1e-9)
	assert.InDelta(t, 3.0, store.Cost("CODEX-MINI", 1_000_000, 1_000_000), 1e-9)
	// Defaults survive for untouched families.
	assert.InDelta(t, 90.0, store.Cost("claude-opus-4-8", 1_000_000, 1_000_000), 1e-9)
}

func TestLongestPrefixWins(t *testing.T) {
	store := NewStore(t.TempDir())
	store.SetPricing(map[string]ModelPrice{
		"claude":          {InputPerMTok: 1, OutputPerMTok: 1},
		"claude-sonnet-4": {InputPerMTok: 10, OutputPerMTok: 10},
	})

	assert.InDelta(t, 20.0, store.Cost("claude-sonnet-4-6", 1_000_000, 1_000_000), 1e-9)
	assert.InDelta(t, 2.0, store.Cost("claude-mystery", 1_000_000, 1_000_000), 1e-9)
}

func TestAnalyticsAggregatesCost(t *testing.T) {
	store := NewStore(t.TempDir())
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	priced := record("CB-1", "claude", true, 1_000_000, 0, 1000, base)
	priced.Model = "claude-haiku-4-5"
	require.NoError(t, store.Append(priced))

	unpriced := record("CB-2", "codex", true, 500, 500, 1000, base.Add(time.Hour))
	unpriced.Model = "mystery"
	require.NoError(t, store.Append(unpriced))

	analytics, err := store.Analytics()
	require.NoError(t, err)
	assert.InDelta(t, 1.0, analytics.CostUSD, 1e-9)
	assert.Equal(t, 1, analytics.UnpricedRuns)
	assert.InDelta(t, 1.0, analytics.ByAgent["claude"].CostUSD, 1e-9)

	records, err := store.Records(0)
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.InDelta(t, 1.0, records[1].CostUSD, 1e-9)
}
