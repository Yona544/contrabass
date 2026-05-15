package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/junhoyeo/contrabass/internal/config"
)

var errDoctorConfigRequired = errors.New("doctor requires --config")

var (
	doctorLookPath   = exec.LookPath
	doctorRunCommand = func(ctx context.Context, cwd string, name string, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = cwd
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	doctorUserHomeDir = os.UserHomeDir
)

type doctorOptions struct {
	ConfigPath string
	RepoPath   string
}

type doctorSeverity string

const (
	doctorPass doctorSeverity = "PASS"
	doctorWarn doctorSeverity = "WARN"
	doctorFail doctorSeverity = "FAIL"
)

type doctorDiagnostic struct {
	Severity doctorSeverity
	Name     string
	Detail   string
}

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run local readiness checks",
		Long:  "Run local readiness checks for an operator machine before starting Contrabass.",
		RunE:  runDoctor,
	}
	cmd.Flags().String("config", "", "path to WORKFLOW.md file")
	cmd.Flags().String("repo", ".", "path to the repository root")
	return cmd
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	cfgPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return fmt.Errorf("getting config flag: %w", err)
	}
	if strings.TrimSpace(cfgPath) == "" {
		return errDoctorConfigRequired
	}

	repoPath, err := cmd.Flags().GetString("repo")
	if err != nil {
		return fmt.Errorf("getting repo flag: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	diagnostics := runDoctorChecks(ctx, doctorOptions{
		ConfigPath: cfgPath,
		RepoPath:   repoPath,
	})

	failures := 0
	warnings := 0
	for _, diagnostic := range diagnostics {
		switch diagnostic.Severity {
		case doctorFail:
			failures++
		case doctorWarn:
			warnings++
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s\n", diagnostic.Severity, diagnostic.Name, diagnostic.Detail)
	}

	if failures > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "readiness checks failed: %d failure(s), %d warning(s)\n", failures, warnings)
		return fmt.Errorf("readiness checks failed: %d failure(s)", failures)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "readiness checks passed: %d check(s), %d warning(s)\n", len(diagnostics), warnings)
	return nil
}

func runDoctorChecks(ctx context.Context, opts doctorOptions) []doctorDiagnostic {
	repoPath := doctorAbsPath(opts.RepoPath)
	diagnostics := []doctorDiagnostic{}

	gitPath, gitErr := doctorLookPath("git")
	if gitErr != nil {
		diagnostics = append(diagnostics, doctorDiagnostic{
			Severity: doctorFail,
			Name:     "git executable",
			Detail:   "git is required for worktree management; install git and ensure it is on PATH",
		})
	} else {
		diagnostics = append(diagnostics, doctorDiagnostic{
			Severity: doctorPass,
			Name:     "git executable",
			Detail:   fmt.Sprintf("found at %s", gitPath),
		})
	}

	diagnostics = append(diagnostics, checkGitRepository(ctx, repoPath, gitErr == nil)...)
	configPath := opts.ConfigPath
	if !filepath.IsAbs(configPath) {
		configPath = doctorResolvePath(repoPath, configPath)
	}
	cfg, cfgOK, configDiagnostics := checkWorkflowConfig(configPath)
	diagnostics = append(diagnostics, configDiagnostics...)
	diagnostics = append(diagnostics, checkWorkflowFixtures(repoPath)...)

	if cfgOK {
		diagnostics = append(diagnostics, checkTrackerConfiguration(repoPath, cfg)...)
		diagnostics = append(diagnostics, checkWritablePath("workspace path", doctorResolvePath(repoPath, filepath.Join(cfg.WorkspaceBaseDir(), "workspaces")))...)
		diagnostics = append(diagnostics, checkRuntimeTools(ctx, cfg)...)
	}
	diagnostics = append(diagnostics, checkWritablePath("temp directory", os.TempDir())...)

	return diagnostics
}

func checkGitRepository(ctx context.Context, repoPath string, gitAvailable bool) []doctorDiagnostic {
	if !gitAvailable {
		return []doctorDiagnostic{{
			Severity: doctorWarn,
			Name:     "repository state",
			Detail:   "skipped because git was not found",
		}}
	}

	output, err := doctorRunCommand(ctx, repoPath, "git", "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(output) != "true" {
		return []doctorDiagnostic{{
			Severity: doctorFail,
			Name:     "repository state",
			Detail:   fmt.Sprintf("%s is not a git work tree; run doctor from the repository root or pass --repo", repoPath),
		}}
	}

	diagnostics := []doctorDiagnostic{{
		Severity: doctorPass,
		Name:     "repository state",
		Detail:   fmt.Sprintf("%s is a git work tree", repoPath),
	}}

	status, statusErr := doctorRunCommand(ctx, repoPath, "git", "status", "--short")
	if statusErr != nil {
		diagnostics = append(diagnostics, doctorDiagnostic{
			Severity: doctorWarn,
			Name:     "repository state",
			Detail:   "could not inspect working tree status; run git status manually before long local runs",
		})
		return diagnostics
	}
	if strings.TrimSpace(status) != "" {
		diagnostics = append(diagnostics, doctorDiagnostic{
			Severity: doctorWarn,
			Name:     "repository state",
			Detail:   "dirty or untracked files are present; preserve unrelated work before long local runs",
		})
		return diagnostics
	}

	diagnostics = append(diagnostics, doctorDiagnostic{
		Severity: doctorPass,
		Name:     "repository state",
		Detail:   "working tree is clean",
	})
	return diagnostics
}

func checkWorkflowConfig(path string) (*config.WorkflowConfig, bool, []doctorDiagnostic) {
	resolved := doctorAbsPath(path)
	if _, err := os.Stat(resolved); err != nil {
		return nil, false, []doctorDiagnostic{{
			Severity: doctorFail,
			Name:     "workflow config",
			Detail:   fmt.Sprintf("cannot read %s: %v", resolved, err),
		}}
	}

	cfg, err := config.ParseWorkflow(resolved)
	if err != nil {
		return nil, false, []doctorDiagnostic{{
			Severity: doctorFail,
			Name:     "workflow config",
			Detail:   fmt.Sprintf("cannot read or parse %s: %v", resolved, err),
		}}
	}

	detail := fmt.Sprintf("parsed %s with tracker.type=%s agent.type=%s", resolved, cfg.TrackerType(), cfg.AgentType())
	if strings.TrimSpace(cfg.PromptTemplate) == "" {
		detail += " (prompt template is empty)"
	}
	return cfg, true, []doctorDiagnostic{{
		Severity: doctorPass,
		Name:     "workflow config",
		Detail:   detail,
	}}
}

func checkWorkflowFixtures(repoPath string) []doctorDiagnostic {
	fixtures := []string{
		filepath.Join(repoPath, "testdata", "workflow.demo.md"),
		filepath.Join(repoPath, "testdata", "workflow.github.md"),
		filepath.Join(repoPath, "testdata", "workflow.local.md"),
	}

	missing := []string{}
	for _, fixture := range fixtures {
		if _, err := os.Stat(fixture); err != nil {
			missing = append(missing, filepath.Base(fixture))
		}
	}
	if len(missing) > 0 {
		return []doctorDiagnostic{{
			Severity: doctorWarn,
			Name:     "workflow fixtures",
			Detail:   fmt.Sprintf("missing %s under testdata; source checkout fixtures may be incomplete", strings.Join(missing, ", ")),
		}}
	}

	return []doctorDiagnostic{{
		Severity: doctorPass,
		Name:     "workflow fixtures",
		Detail:   "testdata workflow fixtures are present",
	}}
}

func checkTrackerConfiguration(repoPath string, cfg *config.WorkflowConfig) []doctorDiagnostic {
	switch cfg.TrackerType() {
	case "internal", "local":
		boardDir := doctorResolvePath(repoPath, cfg.LocalBoardDir())
		diagnostics := []doctorDiagnostic{{
			Severity: doctorPass,
			Name:     "tracker configuration",
			Detail:   fmt.Sprintf("internal board uses prefix %s at %s", cfg.LocalIssuePrefix(), boardDir),
		}}
		if _, err := os.Stat(boardDir); errors.Is(err, os.ErrNotExist) {
			diagnostics = append(diagnostics, doctorDiagnostic{
				Severity: doctorPass,
				Name:     "tracker configuration",
				Detail:   fmt.Sprintf("board directory %s will be created on first board use", boardDir),
			})
		}
		return append(diagnostics, checkWritablePath("tracker storage", boardDir)...)
	case "linear":
		if strings.TrimSpace(linearAPIKey(cfg)) == "" {
			return []doctorDiagnostic{{
				Severity: doctorFail,
				Name:     "tracker configuration",
				Detail:   "LINEAR_API_KEY or tracker.token is not set; set credentials or use tracker.type internal for local-only operation",
			}}
		}
	case "github":
		if strings.TrimSpace(os.Getenv("GITHUB_TOKEN")) == "" && strings.TrimSpace(cfg.GitHubToken()) == "" {
			return []doctorDiagnostic{{
				Severity: doctorFail,
				Name:     "tracker configuration",
				Detail:   "GITHUB_TOKEN or tracker.token is not set; set credentials or use tracker.type internal",
			}}
		}
	default:
		return []doctorDiagnostic{{
			Severity: doctorFail,
			Name:     "tracker configuration",
			Detail:   fmt.Sprintf("unsupported tracker.type %q", cfg.TrackerType()),
		}}
	}

	return []doctorDiagnostic{{
		Severity: doctorPass,
		Name:     "tracker configuration",
		Detail:   fmt.Sprintf("%s tracker has required local configuration", cfg.TrackerType()),
	}}
}

func checkRuntimeTools(ctx context.Context, cfg *config.WorkflowConfig) []doctorDiagnostic {
	required := []runtimeTool{
		{name: "go", action: "install Go 1.25+ and ensure go is on PATH"},
		{name: "node", action: "install Node.js or ensure node is on PATH"},
		{name: "bun", action: "install Bun 1.3+ or run go-only verification"},
	}

	agentTool := agentRuntimeTool(cfg)
	if agentTool.name != "" {
		required = append(required, agentTool)
	}
	if cfg.WorkerMode() == "tmux" {
		required = append(required, runtimeTool{
			name:   "tmux",
			action: "install tmux or set team.worker_mode: goroutine",
		})
	}

	diagnostics := make([]doctorDiagnostic, 0, len(required))
	seen := map[string]bool{}
	for _, tool := range required {
		if seen[tool.name] {
			continue
		}
		seen[tool.name] = true
		path, err := findRuntimeTool(tool.name)
		if err != nil {
			diagnostics = append(diagnostics, doctorDiagnostic{
				Severity: doctorFail,
				Name:     "runtime tool " + tool.name,
				Detail:   tool.action,
			})
			continue
		}
		diagnostic := doctorDiagnostic{
			Severity: doctorPass,
			Name:     "runtime tool " + tool.name,
			Detail:   fmt.Sprintf("found at %s", path),
		}
		if versionArgs, ok := runtimeToolVersionArgs(tool.name); ok {
			output, versionErr := doctorRunCommand(ctx, "", path, versionArgs...)
			version := compactDoctorOutput(output)
			if versionErr != nil {
				diagnostic.Severity = doctorWarn
				diagnostic.Detail = fmt.Sprintf("found at %s, but could not inspect version with %s %s: %v", path, tool.name, strings.Join(versionArgs, " "), versionErr)
				if version != "" {
					diagnostic.Detail += fmt.Sprintf(" (%s)", version)
				}
			} else if version != "" {
				diagnostic.Detail = fmt.Sprintf("found at %s (%s)", path, version)
			}
		}
		diagnostics = append(diagnostics, diagnostic)
	}
	return diagnostics
}

func runtimeToolVersionArgs(name string) ([]string, bool) {
	switch name {
	case "go":
		return []string{"version"}, true
	case "node", "bun":
		return []string{"--version"}, true
	default:
		return nil, false
	}
}

func compactDoctorOutput(output string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(output)), " ")
}

func findRuntimeTool(name string) (string, error) {
	path, err := doctorLookPath(name)
	if err == nil {
		return path, nil
	}
	if name != "bun" {
		return "", err
	}

	for _, candidate := range bunInstallCandidates() {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", err
}

func bunInstallCandidates() []string {
	exeName := "bun"
	if runtimeGOOS() == "windows" {
		exeName = "bun.exe"
	}

	candidates := []string{}
	if bunInstall := strings.TrimSpace(os.Getenv("BUN_INSTALL")); bunInstall != "" {
		candidates = append(candidates, filepath.Join(bunInstall, "bin", exeName))
	}
	if homeDir, err := doctorUserHomeDir(); err == nil && strings.TrimSpace(homeDir) != "" {
		candidates = append(candidates, filepath.Join(homeDir, ".bun", "bin", exeName))
	}
	return candidates
}

func runtimeGOOS() string {
	return runtimeGOOSValue
}

var runtimeGOOSValue = runtime.GOOS

type runtimeTool struct {
	name   string
	action string
}

func agentRuntimeTool(cfg *config.WorkflowConfig) runtimeTool {
	switch cfg.AgentType() {
	case "codex":
		return runtimeTool{name: firstCommandToken(cfg.CodexBinaryPath()), action: "install codex or update codex.binary_path"}
	case "opencode":
		return runtimeTool{name: firstCommandToken(cfg.OpenCodeBinaryPath()), action: "install opencode or update opencode.binary_path"}
	case "oh-my-opencode":
		return runtimeTool{name: firstCommandToken(cfg.OpenCodeBinaryPath()), action: "install oh-my-opencode/opencode or update opencode.binary_path"}
	case "omx":
		return runtimeTool{name: firstCommandToken(cfg.OMXBinaryPath()), action: "install omx or update omx.binary_path"}
	case "omc":
		return runtimeTool{name: firstCommandToken(cfg.OMCBinaryPath()), action: "install omc or update omc.binary_path"}
	default:
		return runtimeTool{name: "", action: ""}
	}
}

func firstCommandToken(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], `"'`)
}

func checkWritablePath(name, target string) []doctorDiagnostic {
	probeDir, targetExists, err := nearestExistingDir(target)
	if err != nil {
		return []doctorDiagnostic{{
			Severity: doctorFail,
			Name:     name,
			Detail:   fmt.Sprintf("cannot find existing parent for %s: %v", target, err),
		}}
	}

	file, err := os.CreateTemp(probeDir, ".contrabass-doctor-*")
	if err != nil {
		return []doctorDiagnostic{{
			Severity: doctorFail,
			Name:     name,
			Detail:   fmt.Sprintf("cannot write under %s for target %s: %v", probeDir, target, err),
		}}
	}
	tempPath := file.Name()
	closeErr := file.Close()
	removeErr := os.Remove(tempPath)
	if closeErr != nil || removeErr != nil {
		return []doctorDiagnostic{{
			Severity: doctorFail,
			Name:     name,
			Detail:   fmt.Sprintf("write probe cleanup failed for %s: close=%v remove=%v", tempPath, closeErr, removeErr),
		}}
	}

	detail := fmt.Sprintf("writable via %s", probeDir)
	if targetExists {
		detail = fmt.Sprintf("%s is writable", target)
	}
	return []doctorDiagnostic{{
		Severity: doctorPass,
		Name:     name,
		Detail:   detail,
	}}
}

func nearestExistingDir(target string) (string, bool, error) {
	candidate := doctorAbsPath(target)
	info, err := os.Stat(candidate)
	if err == nil {
		if !info.IsDir() {
			return "", false, fmt.Errorf("%s exists but is not a directory", candidate)
		}
		return candidate, true, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}

	for {
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", false, os.ErrNotExist
		}
		info, err := os.Stat(parent)
		if err == nil {
			if !info.IsDir() {
				return "", false, fmt.Errorf("%s exists but is not a directory", parent)
			}
			return parent, false, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", false, err
		}
		candidate = parent
	}
}

func doctorResolvePath(repoPath, value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(repoPath, value))
}

func doctorAbsPath(value string) string {
	if strings.TrimSpace(value) == "" {
		value = "."
	}
	if abs, err := filepath.Abs(value); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(value)
}
