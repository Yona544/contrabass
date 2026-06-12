package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// recipeLabelPattern restricts which labels may be used as template file
// names — anything else could escape the recipes directory.
var recipeLabelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// ResolvePromptRecipe returns the contents of the first <label>.md file in
// dir matching the issue's labels (in label order). The returned name is the
// matched label for logging. ok is false when no recipe matches, in which
// case callers fall back to the workflow's prompt template.
func ResolvePromptRecipe(dir string, labels []string) (template string, name string, ok bool) {
	if strings.TrimSpace(dir) == "" {
		return "", "", false
	}

	for _, label := range labels {
		label = strings.ToLower(strings.TrimSpace(label))
		if !recipeLabelPattern.MatchString(label) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, label+".md"))
		if err != nil {
			continue
		}
		trimmed := strings.TrimSpace(string(content))
		if trimmed == "" {
			continue
		}
		return trimmed, label, true
	}
	return "", "", false
}
