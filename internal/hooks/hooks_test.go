package hooks

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name      string
		opts      func(t *testing.T) Options
		assertion func(t *testing.T, err error, opts Options)
	}{
		{
			name: "empty command is no op",
			opts: func(t *testing.T) Options {
				t.Helper()
				return Options{Command: "   "}
			},
			assertion: func(t *testing.T, err error, _ Options) {
				t.Helper()
				require.NoError(t, err)
			},
		},
		{
			name: "runs command with dir and env overrides",
			opts: func(t *testing.T) Options {
				t.Helper()
				return Options{
					Command: successCommand("HOOK_VALUE", "hook.out"),
					Dir:     t.TempDir(),
					Env:     map[string]string{"HOOK_VALUE": "from-env"},
				}
			},
			assertion: func(t *testing.T, err error, opts Options) {
				t.Helper()
				require.NoError(t, err)
				data, readErr := os.ReadFile(filepath.Join(opts.Dir, "hook.out"))
				require.NoError(t, readErr)
				assert.Equal(t, "from-env", strings.TrimSpace(string(data)))
			},
		},
		{
			name: "returns named failure with output",
			opts: func(t *testing.T) Options {
				t.Helper()
				return Options{Name: "before_run", Command: failureCommand("hook failed")}
			},
			assertion: func(t *testing.T, err error, _ Options) {
				t.Helper()
				require.Error(t, err)
				assert.Contains(t, err.Error(), "before_run hook failed")
				assert.Contains(t, err.Error(), "hook failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts(t)
			err := Run(context.Background(), opts)
			tt.assertion(t, err, opts)
		})
	}
}

func TestHookOutputSuffix(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty output", in: " \n\t ", want: ""},
		{name: "keeps trimmed output", in: "\nfailed\n", want: ": failed"},
		{
			name: "truncates long output from the end",
			in:   strings.Repeat("a", maxHookOutput) + "tail",
			want: ": " + strings.Repeat("a", maxHookOutput-4) + "tail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hookOutputSuffix([]byte(tt.in)))
		})
	}
}

func successCommand(envName, outFile string) string {
	if runtime.GOOS == "windows" {
		return "echo %" + envName + "%>" + outFile
	}
	return "printf \"%s\" \"$" + envName + "\" > " + outFile
}

func failureCommand(message string) string {
	if runtime.GOOS == "windows" {
		return "echo " + message + ">&2 && exit /b 7"
	}
	return "echo " + message + " >&2; exit 7"
}
