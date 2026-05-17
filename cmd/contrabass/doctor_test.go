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

	"github.com/junhoyeo/contrabass/internal/config"
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

func TestDoctorCommandAllowsLinearTrackerTokenFallback(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "")
	cfgPath := writeRootWorkflowConfig(t, `---
project_url: https://linear.app/acme/project/local
tracker:
  type: linear
  token: fake-linear-key
agent:
  type: codex
codex:
  binary_path: codex app-server
---
Local operator workflow.
`)

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
	assert.Contains(t, buf.String(), "PASS tracker configuration")
	assert.Contains(t, buf.String(), "linear tracker has required local configuration")
}

func TestDoctorCommandReportsRuntimeToolVersions(t *testing.T) {
	cfgPath := writeRootWorkflowConfig(t, `---
tracker:
  type: internal
---
Local operator workflow.
`)

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
		case name == "go" && strings.Join(args, " ") == "version":
			return "go version go1.25.0 windows/amd64\n", nil
		case name == "node" && strings.Join(args, " ") == "--version":
			return "v24.0.0\n", nil
		case name == "bun" && strings.Join(args, " ") == "--version":
			return "1.3.0\n", nil
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
	assert.Contains(t, output, "PASS runtime tool go")
	assert.Contains(t, output, "go version go1.25.0 windows/amd64")
	assert.Contains(t, output, "PASS runtime tool node")
	assert.Contains(t, output, "v24.0.0")
	assert.Contains(t, output, "PASS runtime tool bun")
	assert.Contains(t, output, "1.3.0")
}

func TestDoctorCommandWarnsWhenRuntimeVersionProbeFails(t *testing.T) {
	cfgPath := writeRootWorkflowConfig(t, `---
tracker:
  type: internal
---
Local operator workflow.
`)

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
		case name == "go" && strings.Join(args, " ") == "version":
			return "go version failed", errors.New("go version failed")
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
	assert.Contains(t, output, "WARN runtime tool go")
	assert.Contains(t, output, "found at go")
	assert.Contains(t, output, "could not inspect version")
	assert.Contains(t, output, "readiness checks passed")
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

func TestAgentRuntimeTool(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.WorkflowConfig
		want runtimeTool
	}{
		{
			name: "default codex",
			cfg:  &config.WorkflowConfig{},
			want: runtimeTool{name: "codex", action: "install codex or update codex.binary_path"},
		},
		{
			name: "opencode",
			cfg: &config.WorkflowConfig{
				Agent:    config.AgentConfig{Type: "opencode"},
				OpenCode: config.OpenCodeConfig{BinaryPath: `"/opt/bin/opencode" serve`},
			},
			want: runtimeTool{name: "/opt/bin/opencode", action: "install opencode or update opencode.binary_path"},
		},
		{
			name: "oh-my-opencode",
			cfg: &config.WorkflowConfig{
				Agent:    config.AgentConfig{Type: "oh-my-opencode"},
				OpenCode: config.OpenCodeConfig{BinaryPath: "oh-my-opencode serve"},
			},
			want: runtimeTool{name: "oh-my-opencode", action: "install oh-my-opencode/opencode or update opencode.binary_path"},
		},
		{
			name: "omx",
			cfg: &config.WorkflowConfig{
				Agent: config.AgentConfig{Type: "omx"},
				OMX:   config.OMXConfig{BinaryPath: "/opt/bin/omx team"},
			},
			want: runtimeTool{name: "/opt/bin/omx", action: "install omx or update omx.binary_path"},
		},
		{
			name: "omc",
			cfg: &config.WorkflowConfig{
				Agent: config.AgentConfig{Type: "omc"},
				OMC:   config.OMCConfig{BinaryPath: "/opt/bin/omc team"},
			},
			want: runtimeTool{name: "/opt/bin/omc", action: "install omc or update omc.binary_path"},
		},
		{
			name: "unknown",
			cfg:  &config.WorkflowConfig{Agent: config.AgentConfig{Type: "custom"}},
			want: runtimeTool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, agentRuntimeTool(tt.cfg))
		})
	}
}

func TestFirstCommandToken(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{name: "blank", command: "  ", want: ""},
		{name: "bare command", command: "codex app-server", want: "codex"},
		{name: "double quoted command", command: `"C:\Program Files\Codex\codex.exe" app-server`, want: `C:\Program Files\Codex\codex.exe`},
		{name: "single quoted command", command: `'/opt/bin/opencode' serve`, want: "/opt/bin/opencode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, firstCommandToken(tt.command))
		})
	}
}

func TestDoctorAbsPath(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	absInput := filepath.Join(cwd, "nested", "..", "target")

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "blank defaults to current directory", value: " ", want: cwd},
		{name: "relative path resolves from current directory", value: filepath.Join("nested", "..", "target"), want: filepath.Join(cwd, "target")},
		{name: "absolute path is cleaned", value: absInput, want: filepath.Join(cwd, "target")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, filepath.Clean(tt.want), doctorAbsPath(tt.value))
		})
	}
}

func TestNearestExistingDir(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "existing")
	require.NoError(t, os.Mkdir(existing, 0o755))
	filePath := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(filePath, []byte("not a directory"), 0o644))

	tests := []struct {
		name         string
		target       string
		wantDir      string
		wantExists   bool
		wantErrMatch string
	}{
		{name: "existing directory", target: existing, wantDir: existing, wantExists: true},
		{name: "missing child uses parent", target: filepath.Join(existing, "child", "leaf"), wantDir: existing, wantExists: false},
		{name: "file target fails", target: filePath, wantErrMatch: "exists but is not a directory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDir, gotExists, err := nearestExistingDir(tt.target)
			if tt.wantErrMatch != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMatch)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, filepath.Clean(tt.wantDir), gotDir)
			assert.Equal(t, tt.wantExists, gotExists)
		})
	}
}

func TestCheckWritablePath(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "existing")
	require.NoError(t, os.Mkdir(existing, 0o755))
	filePath := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(filePath, []byte("not a directory"), 0o644))

	tests := []struct {
		name        string
		target      string
		wantLevel   doctorSeverity
		wantDetails []string
	}{
		{
			name:      "existing directory is writable",
			target:    existing,
			wantLevel: doctorPass,
			wantDetails: []string{
				existing + " is writable",
			},
		},
		{
			name:      "missing child probes existing parent",
			target:    filepath.Join(existing, "child"),
			wantLevel: doctorPass,
			wantDetails: []string{
				"writable via " + existing,
			},
		},
		{
			name:      "file target fails",
			target:    filePath,
			wantLevel: doctorFail,
			wantDetails: []string{
				"cannot find existing parent",
				"exists but is not a directory",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostics := checkWritablePath("test path", tt.target)
			require.Len(t, diagnostics, 1)
			assert.Equal(t, tt.wantLevel, diagnostics[0].Severity)
			assert.Equal(t, "test path", diagnostics[0].Name)
			for _, detail := range tt.wantDetails {
				assert.Contains(t, diagnostics[0].Detail, detail)
			}
		})
	}
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
