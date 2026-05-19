package tracker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsLinearTracker(t *testing.T) {
	t.Parallel()

	linearClient, err := NewLinearClient(LinearConfig{APIKey: "test-api-key"})
	require.NoError(t, err)

	tests := []struct {
		name    string
		tracker Tracker
		want    bool
	}{
		{name: "nil tracker", tracker: nil},
		{name: "non linear tracker", tracker: NewMockTracker()},
		{name: "linear client", tracker: linearClient, want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IsLinearTracker(tt.tracker))
		})
	}
}

func TestLinearClientIsLinearTracker(t *testing.T) {
	t.Parallel()

	client, err := NewLinearClient(LinearConfig{APIKey: "test-api-key"})
	require.NoError(t, err)

	assert.True(t, client.IsLinearTracker())
}
