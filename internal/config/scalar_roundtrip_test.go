package config_test

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

// Every token FormatScalarToken writes must be valid YAML that reads back
// as the same string: Windows paths and notes with backslashes, quotes and
// control characters included (a herdr worktree_pattern such as
// D:\.worktrees\{repo}\{slug} used to be written as an invalid
// double-quoted scalar).
func TestFormatScalarToken_RoundTripsThroughAYAMLParser(t *testing.T) {
	for _, value := range []string{
		`D:\.worktrees\{repo}\{slug}`,
		`C:\Users\x\AppData`,
		`a\x41b`,
		`ends with a backslash \`,
		`say "hi"`,
		`quote \" and backslash`,
		"tab\there",
		"line\nbreak",
		"plain",
		"with: colon",
		"",
	} {
		token := config.FormatScalarToken(value)
		var got string
		if err := yaml.Unmarshal([]byte("k: "+token+"\n"), &struct {
			K *string `yaml:"k"`
		}{K: &got}); err != nil {
			t.Errorf("FormatScalarToken(%q) = %s: not valid YAML: %v", value, token, err)
			continue
		}
		if got != value {
			t.Errorf("FormatScalarToken(%q) = %s reads back as %q", value, token, got)
		}
	}
}
