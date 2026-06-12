package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/junhoyeo/contrabass/internal/config"
	"github.com/junhoyeo/contrabass/internal/tracker"
)

const (
	// maxScanFileBytes is the largest file the scanner will read.
	maxScanFileBytes = 1 << 20
	// scanTitleTextLimit is the maximum length of the comment text embedded
	// in a generated issue title.
	scanTitleTextLimit = 80
	// scanContextLines is how many lines of code context are captured on
	// each side of a finding.
	scanContextLines = 2
)

// scanSkippedDirs are directory names that are never descended into.
// Hidden directories (leading dot) are skipped independently of this set.
var scanSkippedDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"workspaces":   true,
	"dist":         true,
	"vendor":       true,
	".contrabass":  true,
	".gitnexus":    true,
}

// scanBinaryExtensions are file extensions that are assumed to be binary
// (or otherwise not worth scanning) regardless of size.
var scanBinaryExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
	".ico": true, ".webp": true, ".tiff": true, ".svg": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true,
	".wasm": true, ".o": true, ".a": true, ".obj": true, ".lib": true,
	".pyc": true, ".class": true,
	".zip": true, ".tar": true, ".gz": true, ".tgz": true, ".bz2": true,
	".xz": true, ".7z": true, ".rar": true, ".jar": true,
	".pdf": true, ".mp3": true, ".mp4": true, ".mov": true, ".avi": true,
	".mkv": true, ".wav": true, ".ogg": true,
}

// scanCommentTokens are substrings that must appear before a marker for the
// line to be treated as a comment. Deliberately a simple heuristic.
var scanCommentTokens = []string{"//", "#", "/*", "*", "<!--", "--"}

type scanFinding struct {
	Marker  string
	Text    string
	RelPath string
	Line    int
	Context []string
}

func (f scanFinding) title() string {
	text := f.Text
	if text == "" {
		text = "(no description)"
	}
	return fmt.Sprintf("%s: %s", f.Marker, truncateScanText(text, scanTitleTextLimit))
}

func (f scanFinding) sourceRef() string {
	return fmt.Sprintf("Source: %s:%d", f.RelPath, f.Line)
}

func (f scanFinding) descriptionBody() string {
	var b strings.Builder
	b.WriteString(f.Text)
	b.WriteString("\n\n")
	b.WriteString(f.sourceRef())
	b.WriteString("\n\n```\n")
	b.WriteString(strings.Join(f.Context, "\n"))
	b.WriteString("\n```\n")
	return b.String()
}

func newScanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan source comments for TODO/FIXME markers and file them as board issues",
		Long: `Scan a source tree for TODO/FIXME-style comment markers and turn each new
finding into an internal board issue. Findings whose source location or title
already appear on the board are skipped. Without --apply this is a dry run
that only prints what would be created.`,
		RunE: runScan,
	}
	cmd.Flags().String("path", ".", "directory tree to scan")
	cmd.Flags().String("board-dir", "", "override internal board directory")
	cmd.Flags().String("config", "", "path to WORKFLOW.md file")
	cmd.Flags().StringSlice("markers", []string{"TODO", "FIXME", "HACK"}, "comment markers to scan for (case-sensitive)")
	cmd.Flags().String("label", "todo-scan", "label applied to created issues")
	cmd.Flags().Bool("apply", false, "create the issues instead of printing a dry run")
	return cmd
}

func runScan(cmd *cobra.Command, _ []string) error {
	scanPath, err := cmd.Flags().GetString("path")
	if err != nil {
		return fmt.Errorf("getting path flag: %w", err)
	}

	markersRaw, err := cmd.Flags().GetStringSlice("markers")
	if err != nil {
		return fmt.Errorf("getting markers flag: %w", err)
	}

	label, err := cmd.Flags().GetString("label")
	if err != nil {
		return fmt.Errorf("getting label flag: %w", err)
	}

	apply, err := cmd.Flags().GetBool("apply")
	if err != nil {
		return fmt.Errorf("getting apply flag: %w", err)
	}

	markers := make([]string, 0, len(markersRaw))
	for _, marker := range markersRaw {
		if trimmed := strings.TrimSpace(marker); trimmed != "" {
			markers = append(markers, trimmed)
		}
	}
	if len(markers) == 0 {
		return fmt.Errorf("--markers requires at least one non-empty marker")
	}

	boardDir, issuePrefix, err := resolveScanBoard(cmd)
	if err != nil {
		return err
	}

	findings, err := collectScanFindings(scanPath, markers)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if len(findings) == 0 {
		fmt.Fprintf(out, "no %s markers found under %s\n", strings.Join(markers, "/"), scanPath)
		return nil
	}

	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{
		BoardDir:    boardDir,
		IssuePrefix: issuePrefix,
	})

	existing, err := listExistingScanIssues(localTracker)
	if err != nil {
		return err
	}

	fresh := filterTrackedFindings(findings, existing)
	skipped := len(findings) - len(fresh)
	if len(fresh) == 0 {
		fmt.Fprintf(out, "found %d marker(s); all already tracked on the board\n", len(findings))
		return nil
	}

	if !apply {
		fmt.Fprintf(out, "would create %d issue(s) (dry run; pass --apply to create them):\n", len(fresh))
		for i, finding := range fresh {
			fmt.Fprintf(out, "%4d. %s (%s:%d)\n", i+1, finding.title(), finding.RelPath, finding.Line)
		}
		if skipped > 0 {
			fmt.Fprintf(out, "skipping %d already-tracked finding(s)\n", skipped)
		}
		return nil
	}

	var labels []string
	if trimmed := strings.TrimSpace(label); trimmed != "" {
		labels = []string{trimmed}
	}

	for _, finding := range fresh {
		issue, err := localTracker.CreateIssueWithOptions(context.Background(), tracker.LocalIssueCreateOptions{
			Title:       finding.title(),
			Description: finding.descriptionBody(),
			Labels:      labels,
		})
		if err != nil {
			return fmt.Errorf("creating issue for %s:%d: %w", finding.RelPath, finding.Line, err)
		}
		fmt.Fprintf(out, "created %s %s\n", issue.ID, issue.Title)
	}
	fmt.Fprintf(out, "created %d issue(s)\n", len(fresh))
	if skipped > 0 {
		fmt.Fprintf(out, "skipped %d already-tracked finding(s)\n", skipped)
	}
	return nil
}

// resolveScanBoard determines the board directory and issue prefix:
// --board-dir wins, then the workflow config's board dir, then the
// LocalTracker default (.contrabass/board).
func resolveScanBoard(cmd *cobra.Command) (boardDir string, issuePrefix string, err error) {
	boardDir, err = cmd.Flags().GetString("board-dir")
	if err != nil {
		return "", "", fmt.Errorf("getting board-dir flag: %w", err)
	}

	cfgPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return "", "", fmt.Errorf("getting config flag: %w", err)
	}

	if cfgPath != "" {
		cfg, err := config.ParseWorkflow(cfgPath)
		if err != nil {
			return "", "", fmt.Errorf("parsing workflow config: %w", err)
		}
		if boardDir == "" {
			boardDir = cfg.LocalBoardDir()
		}
		issuePrefix = cfg.LocalIssuePrefix()
	}

	return boardDir, issuePrefix, nil
}

// listExistingScanIssues returns all board issues (including done) for
// dedupe. A missing board directory simply means no existing issues; this
// keeps dry runs free of side effects.
func listExistingScanIssues(localTracker *tracker.LocalTracker) ([]tracker.LocalBoardIssue, error) {
	if _, err := os.Stat(localTracker.BoardDir()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("checking board directory %s: %w", localTracker.BoardDir(), err)
	}

	issues, err := localTracker.ListIssues(context.Background(), true)
	if err != nil {
		return nil, fmt.Errorf("listing board issues: %w", err)
	}
	return issues, nil
}

// filterTrackedFindings drops findings whose Source reference already
// appears in an existing issue description, or whose title matches an
// existing issue exactly.
func filterTrackedFindings(findings []scanFinding, existing []tracker.LocalBoardIssue) []scanFinding {
	existingTitles := make(map[string]bool, len(existing))
	for _, issue := range existing {
		existingTitles[issue.Title] = true
	}

	fresh := make([]scanFinding, 0, len(findings))
	for _, finding := range findings {
		if existingTitles[finding.title()] {
			continue
		}

		tracked := false
		source := finding.sourceRef()
		for _, issue := range existing {
			if descriptionHasSource(issue.Description, source) {
				tracked = true
				break
			}
		}
		if tracked {
			continue
		}

		fresh = append(fresh, finding)
	}
	return fresh
}

// descriptionHasSource reports whether source ("Source: path:line") occurs
// in description without being a line-number prefix of a longer reference
// (so "path:1" does not match "path:12").
func descriptionHasSource(description, source string) bool {
	for offset := 0; ; {
		idx := strings.Index(description[offset:], source)
		if idx < 0 {
			return false
		}
		end := offset + idx + len(source)
		if end >= len(description) || description[end] < '0' || description[end] > '9' {
			return true
		}
		offset += idx + 1
	}
}

func collectScanFindings(root string, markers []string) ([]scanFinding, error) {
	var findings []scanFinding

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			if path != root && shouldSkipScanDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if shouldSkipScanFile(d) {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("resolving relative path for %s: %w", path, relErr)
		}

		fileFindings, scanErr := scanFile(path, filepath.ToSlash(rel), markers)
		if scanErr != nil {
			return scanErr
		}
		findings = append(findings, fileFindings...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", root, err)
	}

	return findings, nil
}

func shouldSkipScanDir(name string) bool {
	return scanSkippedDirs[name] || strings.HasPrefix(name, ".")
}

func shouldSkipScanFile(d fs.DirEntry) bool {
	if scanBinaryExtensions[strings.ToLower(filepath.Ext(d.Name()))] {
		return true
	}
	info, err := d.Info()
	if err != nil {
		return true
	}
	return info.Size() > maxScanFileBytes
}

func scanFile(path, rel string, markers []string) ([]scanFinding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		// NUL byte: binary content regardless of extension.
		return nil, nil
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")

	var findings []scanFinding
	for i, line := range lines {
		marker, text, ok := matchScanMarker(line, markers)
		if !ok {
			continue
		}
		findings = append(findings, scanFinding{
			Marker:  marker,
			Text:    text,
			RelPath: rel,
			Line:    i + 1,
			Context: scanContext(lines, i),
		})
	}

	return findings, nil
}

// matchScanMarker reports the first marker found on a comment-looking line.
// A marker must sit on a word boundary, be followed by ':', '(', whitespace
// or end of line, and have a comment token somewhere before it.
func matchScanMarker(line string, markers []string) (marker string, text string, ok bool) {
	for _, candidate := range markers {
		offset := 0
		for {
			idx := strings.Index(line[offset:], candidate)
			if idx < 0 {
				break
			}
			idx += offset
			offset = idx + 1

			if !isScanMarkerBoundary(line, idx, len(candidate)) {
				continue
			}
			if !hasCommentTokenBefore(line, idx) {
				continue
			}
			return candidate, scanCommentText(line[idx+len(candidate):]), true
		}
	}
	return "", "", false
}

func isScanMarkerBoundary(line string, idx, markerLen int) bool {
	if idx > 0 && isScanWordByte(line[idx-1]) {
		return false
	}
	end := idx + markerLen
	if end >= len(line) {
		return true
	}
	switch line[end] {
	case ':', '(', ' ', '\t':
		return true
	}
	return false
}

func isScanWordByte(b byte) bool {
	return b == '_' ||
		(b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z')
}

func hasCommentTokenBefore(line string, idx int) bool {
	prefix := line[:idx]
	for _, token := range scanCommentTokens {
		if strings.Contains(prefix, token) {
			return true
		}
	}
	return false
}

// scanCommentText extracts the human text after a marker, dropping an
// optional "(owner)" attribution, a leading colon, and trailing comment
// terminators.
func scanCommentText(rest string) string {
	text := strings.TrimSpace(rest)
	if strings.HasPrefix(text, "(") {
		if closing := strings.Index(text, ")"); closing >= 0 {
			text = text[closing+1:]
		}
	}
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), ":"))
	for _, suffix := range []string{"*/", "-->"} {
		text = strings.TrimSpace(strings.TrimSuffix(text, suffix))
	}
	return text
}

func scanContext(lines []string, idx int) []string {
	start := idx - scanContextLines
	if start < 0 {
		start = 0
	}
	end := idx + scanContextLines + 1
	if end > len(lines) {
		end = len(lines)
	}
	return slices.Clone(lines[start:end])
}

func truncateScanText(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit-3]) + "..."
}
