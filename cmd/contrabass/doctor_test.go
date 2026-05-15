package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorCommandInternalWorkflowReportsReady(t *testing.T) {
	boardDir := filepath.Join(t.TempDir(), "board")
	workspaceBase := filepath.Join(t.TempDir(), "workspace-base")
	cfgPath := writeRootWorkflowConfig(t, fmt.Sprintf(`---
model: openai/gpt-5-codex
project_url: local://contrabass
tracker:
  type: internal
  board_dir: %q
  issue_prefix: CB
workspace:
  base_dir: %q
agent:
  type: codex
codex:
  binary_path: codex app-server
---
Local operator workflow.
`, boardDir, workspaceBase))

	restoreDoctorTestHooks(t)
	doctorLookPath = func(name string) (string, error) {
		switch name {
		case "git", "go", "node", "bun", "codex":
			return name, nil
		default:
			return "", exec.ErrNotFound
		}
	}
	doctorRunCommand = func(_ context.Context, _ string, name string, args ...string) (string, error) {
		switch {
		case name == "git" && strings.Join(args, " ") == "rev-parse --is-inside-work-tree":
			return "true\n", nil
		case name == "git" && strings.Join(args, " ") == "status --short":
			return "?? .agents/\n", nil
		default:
			return "", nil
		}
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"doctor", "--config", cfgPath, "--repo", filepath.Dir(cfgPath)})

	require.NoError(t, cmd.Execute())
	output := buf.String()
	assert.Contains(t, output, "PASS git executable")
	assert.Contains(t, output, "PASS workflow config")
	assert.Contains(t, output, "PASS tracker configuration")
	assert.Contains(t, output, "PASS runtime tool codex")
	assert.Contains(t, output, "WARN repository state")
	assert.Contains(t, output, "dirty or untracked files")
	assert.Contains(t, output, "readiness checks passed")
}

func TestDoctorCommandFailsForMissingConfig(t *testing.T) {
	restoreDoctorTestHooks(t)
	doctorLookPath = func(name string) (string, error) {
		if name == "git" {
			return name, nil
		}
		return "", exec.ErrNotFound
	}
	doctorRunCommand = func(_ context.Context, _ string, name string, args ...string) (string, error) {
		if name == "git" && strings.Join(args, " ") == "rev-parse --is-inside-work-tree" {
			return "true\n", nil
		}
		return "", nil
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"doctor", "--config", filepath.Join(t.TempDir(), "missing.md"), "--repo", t.TempDir()})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorContains(t, err, "readiness checks failed")
	assert.Contains(t, buf.String(), "FAIL workflow config")
	assert.Contains(t, buf.String(), "cannot read")
}

func TestDoctorCommandResolvesRelativeConfigFromRepoFlag(t *testing.T) {
	repoDir := t.TempDir()
	cfgPath := filepath.Join(repoDir, "WORKFLOW.md")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`---
tracker:
  type: internal
agent:
  type: codex
codex:
  binary_path: codex app-server
---
Local operator workflow.
`), 0o644))
	t.Chdir(t.TempDir())

	restoreDoctorTestHooks(t)
	doctorLookPath = func(name string) (string, error) {
		switch name {
		case "git", "go", "node", "bun", "codex":
			return name, nil
		default:
			return "", exec.ErrNotFound
		}
	}
	doctorRunCommand = func(_ context.Context, _ string, name string, args ...string) (string, error) {
		switch {
		case name == "git" && strings.Join(args, " ") == "rev-parse --is-inside-work-tree":
			return "true\n", nil
		case name == "git" && strings.Join(args, " ") == "status --short":
			return "", nil
		default:
			return "", nil
		}
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"doctor", "--config", "WORKFLOW.md", "--repo", repoDir})

	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "PASS workflow config")
	assert.Contains(t, buf.String(), cfgPath)
}

func TestDoctorCommandFailsWhenAgentRuntimeMissing(t *testing.T) {
	cfgPath := writeRootWorkflowConfig(t, `---
model: openai/gpt-5-codex
project_url: local://contrabass
tracker:
  type: internal
agent:
  type: opencode
opencode:
  binary_path: opencode serve
---
Local operator workflow.
`)

	restoreDoctorTestHooks(t)
	doctorLookPath = func(name string) (string, error) {
		switch name {
		case "git", "go", "node", "bun":
			return name, nil
		default:
			return "", exec.ErrNotFound
		}
	}
	doctorRunCommand = func(_ context.Context, _ string, name string, args ...string) (string, error) {
		switch {
		case name == "git" && strings.Join(args, " ") == "rev-parse --is-inside-work-tree":
			return "true\n", nil
		case name == "git" && strings.Join(args, " ") == "status --short":
			return "", nil
		default:
			return "", nil
		}
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"doctor", "--config", cfgPath, "--repo", filepath.Dir(cfgPath)})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorContains(t, err, "readiness checks failed")
	assert.Contains(t, buf.String(), "FAIL runtime tool opencode")
	assert.Contains(t, buf.String(), "install opencode or update opencode.binary_path")
}

func TestDoctorCommandFindsBunUserInstallWhenBunIsNotOnPath(t *testing.T) {
	t.Setenv("BUN_INSTALL", "")

	cfgPath := writeRootWorkflowConfig(t, `---
tracker:
  type: internal
---
Local operator workflow.
`)

	homeDir := t.TempDir()
	bunExe := "bun"
	if runtime.GOOS == "windows" {
		bunExe = "bun.exe"
	}
	bunPath := filepath.Join(homeDir, ".bun", "bin", bunExe)
	require.NoError(t, os.MkdirAll(filepath.Dir(bunPath), 0o755))
	require.NoError(t, os.WriteFile(bunPath, []byte("fake bun"), 0o755))

	restoreDoctorTestHooks(t)
	doctorUserHomeDir = func() (string, error) {
		return homeDir, nil
	}
	doctorLookPath = func(name string) (string, error) {
		switch name {
		case "git", "go", "node", "codex":
			return name, nil
		case "bun":
			return "", exec.ErrNotFound
		default:
			return "", exec.ErrNotFound
		}
	}
	doctorRunCommand = func(_ context.Context, _ string, name string, args ...string) (string, error) {
		switch {
		case name == "git" && strings.Join(args, " ") == "rev-parse --is-inside-work-tree":
			return "true\n", nil
		case name == "git" && strings.Join(args, " ") == "status --short":
			return "", nil
		default:
			return "", nil
		}
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"doctor", "--config", cfgPath, "--repo", filepath.Dir(cfgPath)})

	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "PASS runtime tool bun")
	assert.Contains(t, buf.String(), bunPath)
}

func restoreDoctorTestHooks(t *testing.T) {
	t.Helper()

	originalLookPath := doctorLookPath
	originalRunCommand := doctorRunCommand
	originalUserHomeDir := doctorUserHomeDir

	t.Cleanup(func() {
		doctorLookPath = originalLookPath
		doctorRunCommand = originalRunCommand
		doctorUserHomeDir = originalUserHomeDir
	})
	doctorUserHomeDir = func() (string, error) {
		return t.TempDir(), nil
	}
}

func TestDoctorCommandRejectsMissingConfigFlag(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"doctor"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.True(t, errors.Is(err, errDoctorConfigRequired))
}

func TestDoctorCommandHelpMentionsReadiness(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"doctor", "--help"})

	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "local readiness")
	assert.Contains(t, buf.String(), "--config")
	assert.Contains(t, buf.String(), "--repo")
}

func TestDoctorCommandDoesNotCreateConfiguredBoardDirectory(t *testing.T) {
	boardDir := filepath.Join(t.TempDir(), "board")
	cfgPath := writeRootWorkflowConfig(t, fmt.Sprintf(`---
tracker:
  type: internal
  board_dir: %q
---
Prompt.
`, boardDir))

	restoreDoctorTestHooks(t)
	doctorLookPath = func(name string) (string, error) {
		switch name {
		case "git", "go", "node", "bun", "codex":
			return name, nil
		default:
			return "", exec.ErrNotFound
		}
	}
	doctorRunCommand = func(_ context.Context, _ string, name string, args ...string) (string, error) {
		switch {
		case name == "git" && strings.Join(args, " ") == "rev-parse --is-inside-work-tree":
			return "true\n", nil
		case name == "git" && strings.Join(args, " ") == "status --short":
			return "", nil
		default:
			return "", nil
		}
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"doctor", "--config", cfgPath, "--repo", filepath.Dir(cfgPath)})

	require.NoError(t, cmd.Execute())
	_, statErr := os.Stat(boardDir)
	assert.True(t, errors.Is(statErr, os.ErrNotExist), "doctor should not initialize the board")
	assert.Contains(t, buf.String(), "will be created on first board use")
}

func TestCheckWorkflowFixturesIncludesLocalWorkflow(t *testing.T) {
	repoDir := t.TempDir()
	testdataDir := filepath.Join(repoDir, "testdata")
	require.NoError(t, os.MkdirAll(testdataDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(testdataDir, "workflow.demo.md"), []byte("demo"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(testdataDir, "workflow.github.md"), []byte("github"), 0o644))

	diagnostics := checkWorkflowFixtures(repoDir)

	require.Len(t, diagnostics, 1)
	assert.Equal(t, doctorWarn, diagnostics[0].Severity)
	assert.Contains(t, diagnostics[0].Detail, "workflow.local.md")
}
