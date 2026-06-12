package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/junhoyeo/contrabass/internal/tracker"
)

func runScanCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := newScanCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func writeScanTestFile(t *testing.T, root, rel, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func scanTestBoardDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "board")
}

func TestScanDetectsMarkersAcrossCommentStyles(t *testing.T) {
	root := t.TempDir()
	writeScanTestFile(t, root, "main.go", "package main\n\n// TODO: fix go thing\nfunc main() {}\n")
	writeScanTestFile(t, root, "script.py", "import os\n# FIXME broken pipe\nprint(1)\n")
	writeScanTestFile(t, root, "block.c", "/*\n * TODO: block middle\n */\nint x; /* HACK: temporary workaround */\n")
	writeScanTestFile(t, root, "owner.py", "# FIXME(yona): attributed finding\n")
	writeScanTestFile(t, root, "notes.txt", "TODO: not a comment so not a finding\n")

	stdout, err := runScanCmd(t, "--path", root, "--board-dir", scanTestBoardDir(t))
	require.NoError(t, err)

	assert.Contains(t, stdout, "would create 5 issue(s)")
	assert.Contains(t, stdout, "TODO: fix go thing (main.go:3)")
	assert.Contains(t, stdout, "FIXME: broken pipe (script.py:2)")
	assert.Contains(t, stdout, "TODO: block middle (block.c:2)")
	assert.Contains(t, stdout, "HACK: temporary workaround (block.c:4)")
	assert.Contains(t, stdout, "FIXME: attributed finding (owner.py:1)")
	assert.NotContains(t, stdout, "not a comment")
}

func TestScanSkipsVendoredHiddenAndBinaryPaths(t *testing.T) {
	root := t.TempDir()
	writeScanTestFile(t, root, "src/app.go", "// TODO: real finding\n")
	writeScanTestFile(t, root, "node_modules/dep/index.js", "// TODO: vendored\n")
	writeScanTestFile(t, root, ".git/config", "# TODO: git internal\n")
	writeScanTestFile(t, root, ".hidden/file.go", "// TODO: hidden dir\n")
	writeScanTestFile(t, root, "vendor/lib/lib.go", "// TODO: vendor dir\n")
	writeScanTestFile(t, root, "logo.png", "// TODO: binary extension\n")
	writeScanTestFile(t, root, "huge.go", "// TODO: huge file\n"+strings.Repeat("x", maxScanFileBytes+1))
	writeScanTestFile(t, root, "blob.dat", "// TODO: nul content\n\x00binary\n")

	stdout, err := runScanCmd(t, "--path", root, "--board-dir", scanTestBoardDir(t))
	require.NoError(t, err)

	assert.Contains(t, stdout, "would create 1 issue(s)")
	assert.Contains(t, stdout, "TODO: real finding (src/app.go:1)")
	assert.NotContains(t, stdout, "vendored")
	assert.NotContains(t, stdout, "git internal")
	assert.NotContains(t, stdout, "hidden dir")
	assert.NotContains(t, stdout, "vendor dir")
	assert.NotContains(t, stdout, "binary extension")
	assert.NotContains(t, stdout, "huge file")
	assert.NotContains(t, stdout, "nul content")
}

func TestScanDedupesAgainstExistingBoardIssues(t *testing.T) {
	root := t.TempDir()
	writeScanTestFile(t, root, "tracked.go", "// TODO: already tracked by source\n")
	writeScanTestFile(t, root, "titled.go", "// TODO: duplicate title\n")
	writeScanTestFile(t, root, "new.go", "// TODO: brand new finding\n")

	boardDir := scanTestBoardDir(t)
	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{BoardDir: boardDir})
	_, err := localTracker.CreateIssueWithOptions(context.Background(), tracker.LocalIssueCreateOptions{
		Title:       "filed earlier with a different title",
		Description: "already on the board\n\nSource: tracked.go:1\n",
	})
	require.NoError(t, err)
	_, err = localTracker.CreateIssueWithOptions(context.Background(), tracker.LocalIssueCreateOptions{
		Title:       "TODO: duplicate title",
		Description: "same title, no source reference",
	})
	require.NoError(t, err)

	stdout, err := runScanCmd(t, "--path", root, "--board-dir", boardDir)
	require.NoError(t, err)

	assert.Contains(t, stdout, "would create 1 issue(s)")
	assert.Contains(t, stdout, "TODO: brand new finding (new.go:1)")
	assert.NotContains(t, stdout, "already tracked by source")
	assert.NotContains(t, stdout, "duplicate title")
	assert.Contains(t, stdout, "skipping 2 already-tracked finding(s)")
}

func TestScanDryRunDoesNotCreateIssues(t *testing.T) {
	root := t.TempDir()
	writeScanTestFile(t, root, "app.go", "// TODO: dry run finding\n")

	boardDir := scanTestBoardDir(t)
	stdout, err := runScanCmd(t, "--path", root, "--board-dir", boardDir)
	require.NoError(t, err)

	assert.Contains(t, stdout, "would create 1 issue(s)")
	_, statErr := os.Stat(boardDir)
	assert.True(t, os.IsNotExist(statErr), "dry run must not create the board directory")
}

func TestScanApplyCreatesIssuesWithLabel(t *testing.T) {
	root := t.TempDir()
	writeScanTestFile(t, root, "a.go", "package a\n\n// TODO: first finding\nvar x = 1\n")
	writeScanTestFile(t, root, "b.py", "# FIXME: second finding\n")

	boardDir := scanTestBoardDir(t)
	stdout, err := runScanCmd(t, "--path", root, "--board-dir", boardDir, "--label", "custom-label", "--apply")
	require.NoError(t, err)
	assert.Contains(t, stdout, "created 2 issue(s)")
	assert.NotContains(t, stdout, "would create")

	localTracker := tracker.NewLocalTracker(tracker.LocalConfig{BoardDir: boardDir})
	issues, err := localTracker.ListIssues(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, issues, 2)

	titles := make([]string, 0, len(issues))
	for _, issue := range issues {
		titles = append(titles, issue.Title)
		assert.Equal(t, []string{"custom-label"}, issue.Labels)
		assert.Contains(t, stdout, "created "+issue.ID)
	}
	assert.Contains(t, titles, "TODO: first finding")
	assert.Contains(t, titles, "FIXME: second finding")

	for _, issue := range issues {
		if issue.Title != "TODO: first finding" {
			continue
		}
		assert.Contains(t, issue.Description, "first finding\n")
		assert.Contains(t, issue.Description, "Source: a.go:3")
		assert.Contains(t, issue.Description, "```\npackage a\n\n// TODO: first finding\nvar x = 1\n\n```")
	}

	// A second apply run must dedupe everything it just created.
	rerun, err := runScanCmd(t, "--path", root, "--board-dir", boardDir, "--apply")
	require.NoError(t, err)
	assert.Contains(t, rerun, "all already tracked")

	issues, err = localTracker.ListIssues(context.Background(), true)
	require.NoError(t, err)
	assert.Len(t, issues, 2)
}

func TestScanMarkersFlagOverride(t *testing.T) {
	root := t.TempDir()
	writeScanTestFile(t, root, "main.go", "// TODO: ignored default marker\n// XXX: custom marker hit\n// xxx: lowercase is not matched\n")

	stdout, err := runScanCmd(t, "--path", root, "--board-dir", scanTestBoardDir(t), "--markers", "XXX")
	require.NoError(t, err)

	assert.Contains(t, stdout, "would create 1 issue(s)")
	assert.Contains(t, stdout, "XXX: custom marker hit (main.go:2)")
	assert.NotContains(t, stdout, "ignored default marker")
	assert.NotContains(t, stdout, "lowercase")
}

func TestScanTitleTruncation(t *testing.T) {
	root := t.TempDir()
	longText := strings.Repeat("a", 120)
	writeScanTestFile(t, root, "long.go", "// TODO: "+longText+"\n")

	stdout, err := runScanCmd(t, "--path", root, "--board-dir", scanTestBoardDir(t))
	require.NoError(t, err)

	assert.Contains(t, stdout, "would create 1 issue(s)")
	assert.Contains(t, stdout, "TODO: "+strings.Repeat("a", 77)+"... (long.go:1)")
	assert.NotContains(t, stdout, strings.Repeat("a", 78))
}

func TestScanReportsZeroFindings(t *testing.T) {
	root := t.TempDir()
	writeScanTestFile(t, root, "clean.go", "package clean\n")

	boardDir := scanTestBoardDir(t)
	stdout, err := runScanCmd(t, "--path", root, "--board-dir", boardDir)
	require.NoError(t, err)

	assert.Contains(t, stdout, "no TODO/FIXME/HACK markers found")
	_, statErr := os.Stat(boardDir)
	assert.True(t, os.IsNotExist(statErr))
}

func TestScanBareMarkerWithoutColonOrSuffixIsDetected(t *testing.T) {
	root := t.TempDir()
	writeScanTestFile(t, root, "bare.go", "// TODO\nvar mastodon = 1 // TODONT is not a marker\n")

	stdout, err := runScanCmd(t, "--path", root, "--board-dir", scanTestBoardDir(t))
	require.NoError(t, err)

	assert.Contains(t, stdout, "would create 1 issue(s)")
	assert.Contains(t, stdout, "TODO: (no description) (bare.go:1)")
	assert.NotContains(t, stdout, "TODONT")
}
