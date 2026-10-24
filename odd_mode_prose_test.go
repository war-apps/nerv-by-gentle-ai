package nerv

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// NERV runs on gentle-ai's ODD feature document instead of OpenSpec
// (odd/tasks/ship-deploy-plugins.md, T5, S10). Each test pins one rule.

const oddDocumentRef = "skills/nerv-orchestrator/references/odd-document.md"

var openspecWord = regexp.MustCompile(`(?i)openspec`)

type openspecHit struct{ path, line string }

func openspecHits(t *testing.T, fsys fs.FS) []openspecHit {
	t.Helper()
	var hits []openspecHit
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if openspecWord.MatchString(line) {
				hits = append(hits, openspecHit{path, line})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the plugin tree: %v", err)
	}
	return hits
}

// The only mention of OpenSpec left in the core plugin is the one sentence
// in the ODD reference saying it is no longer used.
func TestPlugin_MentionsOpenSpecOnlyToSayItIsNoLongerUsed(t *testing.T) {
	hits := openspecHits(t, PluginFS())
	if len(hits) != 1 {
		t.Fatalf("plugin/ mentions openspec on %d lines, want exactly 1:\n%v", len(hits), hits)
	}
	if hits[0].path != oddDocumentRef || !strings.Contains(hits[0].line, "no longer used") {
		t.Errorf("the one openspec mention is %s: %q, want the no-longer-used sentence in %s", hits[0].path, hits[0].line, oddDocumentRef)
	}
}

// The extension plugins never mention OpenSpec: their runtime files live in
// the ODD run folder too.
func TestExtensions_NeverMentionOpenSpec(t *testing.T) {
	if hits := openspecHits(t, ExtensionsFS()); len(hits) != 0 {
		t.Errorf("plugins/ still mentions openspec:\n%v", hits)
	}
}

// The orchestrator names the ODD document as the tracking surface.
func TestOrchestrator_NamesTheODDDocument(t *testing.T) {
	assertContainsAll(t, orchestratorCore, readPluginText(t, orchestratorCore), "odd/tasks/{feature}.md")
}

// The orchestrator keeps the per-run governance files in odd/<feature>/nerv/.
func TestOrchestrator_NamesTheRunFolder(t *testing.T) {
	assertContainsAll(t, orchestratorCore, readPluginText(t, orchestratorCore), "odd/{feature}/nerv/")
}

// The orchestrator lock lives in the runtime folder, per feature.
func TestOrchestrator_LockLivesUnderNervLocks(t *testing.T) {
	assertContainsAll(t, orchestratorCore, readPluginText(t, orchestratorCore), ".nerv/locks/{feature}.lock")
}

// The ODD reference is named in the core, so it loads.
func TestOrchestrator_NamesTheODDReference(t *testing.T) {
	assertContainsAll(t, orchestratorCore, readPluginText(t, orchestratorCore), "references/odd-document.md")
}

// Resume follows gentle-ai's ODD order.
func TestResume_FollowsTheODDOrder(t *testing.T) {
	const resume = "skills/nerv-orchestrator/references/resume.md"
	text := normalizeSpace(readPluginText(t, resume))
	order := []string{"`mem_context`", "`mem_search`", "`mem_get_observation`", "`odd/tasks/{feature}.md`"}
	last := -1
	for _, step := range order {
		i := strings.Index(text, step)
		if i < 0 {
			t.Fatalf("%s does not name %s", resume, step)
		}
		if i < last {
			t.Errorf("%s names %s out of the ODD resume order %v", resume, step, order)
		}
		last = i
	}
}

// No archive step and no spec composition remain anywhere in the plugin.
func TestPlugin_HasNoArchiveOrSpecCompose(t *testing.T) {
	banned := regexp.MustCompile(`spec-compose|changes/archive|Archive duty|RENAMED Requirements|MODIFIED Requirements`)
	err := fs.WalkDir(PluginFS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(PluginFS(), path)
		if err != nil {
			return err
		}
		if m := banned.FindString(string(data)); m != "" {
			t.Errorf("%s still carries %q", path, m)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the plugin tree: %v", err)
	}
}
