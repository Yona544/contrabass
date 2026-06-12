package approval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMissingIsNone(t *testing.T) {
	store := NewStore(t.TempDir())
	status, plan, err := store.Get("CB-1")
	require.NoError(t, err)
	assert.Equal(t, StatusNone, status)
	assert.Empty(t, plan)
}

func TestPlannedThenApprovedKeepsPlan(t *testing.T) {
	store := NewStore(t.TempDir())

	require.NoError(t, store.MarkPlanned("CB-1", "1. do x\n2. do y"))
	status, plan, err := store.Get("CB-1")
	require.NoError(t, err)
	assert.Equal(t, StatusPlanned, status)
	assert.Contains(t, plan, "do x")

	require.NoError(t, store.Approve("CB-1"))
	status, plan, err = store.Get("CB-1")
	require.NoError(t, err)
	assert.Equal(t, StatusApproved, status)
	assert.Contains(t, plan, "do x", "plan survives approval")
}

func TestPreApprovalWithoutPlan(t *testing.T) {
	store := NewStore(t.TempDir())

	require.NoError(t, store.Approve("CB-2"))
	status, plan, err := store.Get("CB-2")
	require.NoError(t, err)
	assert.Equal(t, StatusApproved, status)
	assert.Empty(t, plan)
}

func TestResetReturnsToNone(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.MarkPlanned("CB-3", "plan"))
	require.NoError(t, store.Reset("CB-3"))

	status, _, err := store.Get("CB-3")
	require.NoError(t, err)
	assert.Equal(t, StatusNone, status)

	require.NoError(t, store.Reset("CB-404"), "resetting unknown issues is a no-op")
}

func TestPendingListsPlannedOnly(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.MarkPlanned("CB-1", "plan one"))
	require.NoError(t, store.MarkPlanned("CB-2", "plan two"))
	require.NoError(t, store.Approve("CB-2"))

	pending, err := store.Pending()
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "CB-1", pending[0].IssueID)

	empty := NewStore(t.TempDir())
	pending, err = empty.Pending()
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestPathSanitizesIssueIDs(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.MarkPlanned("../evil/CB-1", "plan"))

	status, _, err := store.Get("../evil/CB-1")
	require.NoError(t, err)
	assert.Equal(t, StatusPlanned, status)
}
