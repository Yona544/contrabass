package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const maxHookOutput = 4096

type Options struct {
	Name    string
	Command string
	Dir     string
	Env     map[string]string
}

func Run(ctx context.Context, opts Options) error {
	command := strings.TrimSpace(opts.Command)
	if command == "" {
		return nil
	}

	cmd := shellCommand(ctx, command)
	cmd.Dir = opts.Dir
	cmd.Env = mergeEnv(os.Environ(), opts.Env)

	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "workflow"
	}
	return fmt.Errorf("%s hook failed: %w%s", name, err, hookOutputSuffix(output))
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd.exe", "/C", command)
	}
	return exec.CommandContext(ctx, "/bin/sh", "-c", command)
}

func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}

	env := make([]string, 0, len(base)+len(overrides))
	env = append(env, base...)
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env
}

func hookOutputSuffix(output []byte) string {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return ""
	}

	if len(text) > maxHookOutput {
		text = text[len(text)-maxHookOutput:]
	}
	return ": " + text
}
