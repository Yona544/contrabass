package notify

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDesktopCommandPerPlatform(t *testing.T) {
	name, args, ok := desktopCommand("windows", "contrabass · CB-1", "it's done")
	require.True(t, ok)
	assert.Equal(t, "powershell.exe", name)
	require.Len(t, args, 4)
	assert.Equal(t, "-NoProfile", args[0])
	assert.Contains(t, args[3], "contrabass · CB-1")
	assert.Contains(t, args[3], "it''s done", "single quotes must be doubled for PowerShell")

	name, args, ok = desktopCommand("darwin", "title", "msg")
	require.True(t, ok)
	assert.Equal(t, "osascript", name)
	assert.Contains(t, args[1], `display notification "msg" with title "title"`)

	name, args, ok = desktopCommand("linux", "title", "msg")
	require.True(t, ok)
	assert.Equal(t, "notify-send", name)
	assert.Equal(t, []string{"title", "msg"}, args)

	_, _, ok = desktopCommand("plan9", "title", "msg")
	assert.False(t, ok)
}

func TestNotifierDesktopSink(t *testing.T) {
	var mu sync.Mutex
	var capturedName string
	var capturedArgs []string
	original := execDesktopCommand
	execDesktopCommand = func(name string, args ...string) error {
		mu.Lock()
		defer mu.Unlock()
		capturedName = name
		capturedArgs = args
		return nil
	}
	t.Cleanup(func() { execDesktopCommand = original })

	n := New(Config{Desktop: true})
	require.True(t, n.Enabled(), "desktop alone enables the notifier")

	n.Notify(finishedEvent("CB-5", ""))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go n.Start(ctx)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return capturedName != ""
	}, 5*time.Second, 10*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	joined := ""
	for _, arg := range capturedArgs {
		joined += arg + " "
	}
	assert.Contains(t, joined, "CB-5")
}
