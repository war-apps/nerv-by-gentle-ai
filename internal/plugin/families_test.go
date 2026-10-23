package plugin_test

import (
	"slices"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
)

// S1: deploy requires ship; the deploy providers require deploy; the task
// providers require task.
func TestExtensions_EmbeddedRequirements(t *testing.T) {
	exts, err := plugin.Extensions(nerv.ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"task": nil, "task-teamwork": {"task"},
		"ship": nil, "deploy": {"ship"}, "deploy-github": {"deploy"}, "deploy-ssh-compose": {"deploy"},
	}
	for _, e := range exts {
		req, ok := want[e.Name]
		if !ok {
			continue
		}
		if !slices.Equal(e.Requires, req) {
			t.Errorf("%s requires %v, want %v", e.Name, e.Requires, req)
		}
		delete(want, e.Name)
	}
	if len(want) != 0 {
		t.Errorf("not shipped: %v", want)
	}
}

// S1: the suggestion order is task and its providers, then ship, then
// deploy, then the deploy providers; anything else after, by name.
func TestSuggestionOrder(t *testing.T) {
	exts, err := plugin.Extensions(nerv.ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range plugin.SuggestionOrder(exts) {
		got = append(got, e.Name)
	}
	want := []string{"task", "task-teamwork", "ship", "deploy", "deploy-github", "deploy-ssh-compose"}
	if !slices.Equal(got, want) {
		t.Errorf("SuggestionOrder = %v, want %v", got, want)
	}
	other := []plugin.Extension{{Name: "zeta"}, {Name: "deploy", Requires: []string{"ship"}}, {Name: "alpha"}, {Name: "ship"}}
	got = nil
	for _, e := range plugin.SuggestionOrder(other) {
		got = append(got, e.Name)
	}
	if want := []string{"ship", "deploy", "alpha", "zeta"}; !slices.Equal(got, want) {
		t.Errorf("SuggestionOrder = %v, want %v", got, want)
	}
}
