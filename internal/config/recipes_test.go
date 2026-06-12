package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeRecipe(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

func TestResolvePromptRecipeMatchesFirstLabel(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "bugfix.md", "Fix the bug: {{ issue.title }}")
	writeRecipe(t, dir, "tests.md", "Write tests for: {{ issue.title }}")

	template, name, ok := ResolvePromptRecipe(dir, []string{"frontend", "bugfix", "tests"})
	require.True(t, ok)
	assert.Equal(t, "bugfix", name)
	assert.Contains(t, template, "Fix the bug")
}

func TestResolvePromptRecipeCaseInsensitiveLabels(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "bugfix.md", "body")

	_, name, ok := ResolvePromptRecipe(dir, []string{"BugFix"})
	require.True(t, ok)
	assert.Equal(t, "bugfix", name)
}

func TestResolvePromptRecipeNoMatch(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "bugfix.md", "body")

	_, _, ok := ResolvePromptRecipe(dir, []string{"feature"})
	assert.False(t, ok)

	_, _, ok = ResolvePromptRecipe(dir, nil)
	assert.False(t, ok)

	_, _, ok = ResolvePromptRecipe("", []string{"bugfix"})
	assert.False(t, ok)
}

func TestResolvePromptRecipeRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Dir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(parent, "evil.md"), []byte("evil"), 0o644))

	_, _, ok := ResolvePromptRecipe(dir, []string{"../evil", "..\\evil", "a/b"})
	assert.False(t, ok)
}

func TestResolvePromptRecipeSkipsEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "bugfix.md", "   \n  ")
	writeRecipe(t, dir, "tests.md", "real body")

	template, name, ok := ResolvePromptRecipe(dir, []string{"bugfix", "tests"})
	require.True(t, ok)
	assert.Equal(t, "tests", name)
	assert.Equal(t, "real body", template)
}
