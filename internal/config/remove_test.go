package config_test

import (
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

const removeFixture = `tasks:
  provider: jira                    # the old tracker
  rounding_minutes: 30
  providers:
    jira:
      site: acme
      # a comment inside jira
      board: 7
    teamwork:
      assignee_id: 42
git:
  base_branch: develop
`

func TestRemove_DeletesAScalarLineAndKeepsEveryOtherByte(t *testing.T) {
	doc := config.Parse(removeFixture)
	if !doc.Remove("tasks.rounding_minutes") {
		t.Fatal("Remove(tasks.rounding_minutes) = false, want true")
	}
	want := `tasks:
  provider: jira                    # the old tracker
  providers:
    jira:
      site: acme
      # a comment inside jira
      board: 7
    teamwork:
      assignee_id: 42
git:
  base_branch: develop
`
	if got := doc.String(); got != want {
		t.Errorf("after Remove:\n%s\nwant:\n%s", got, want)
	}
}

func TestRemove_DeletesANestedBlockWithItsChildrenAndComments(t *testing.T) {
	doc := config.Parse(removeFixture)
	if !doc.Remove("tasks.providers.jira") {
		t.Fatal("Remove(tasks.providers.jira) = false, want true")
	}
	want := `tasks:
  provider: jira                    # the old tracker
  rounding_minutes: 30
  providers:
    teamwork:
      assignee_id: 42
git:
  base_branch: develop
`
	if got := doc.String(); got != want {
		t.Errorf("after Remove:\n%s\nwant:\n%s", got, want)
	}
}

func TestRemove_MissingPathChangesNothing(t *testing.T) {
	doc := config.Parse(removeFixture)
	for _, path := range []string{"tasks.nope", "nope", "tasks.providers.github-projects", "git.base_branch.x"} {
		if doc.Remove(path) {
			t.Errorf("Remove(%s) = true, want false", path)
		}
	}
	if doc.String() != removeFixture {
		t.Errorf("a missing path changed the document:\n%s", doc.String())
	}
}

func TestRemove_TopLevelKeyAndCRLF(t *testing.T) {
	doc := config.Parse("a: 1\r\nmodels:\r\n  misato: { model: fable }\r\nb: 2\r\n")
	if !doc.Remove("models") {
		t.Fatal("Remove(models) = false, want true")
	}
	if got, want := doc.String(), "a: 1\r\nb: 2\r\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
