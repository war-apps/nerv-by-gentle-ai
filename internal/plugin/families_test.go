package plugin_test

import (
	"slices"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/plugin"
)

// S1: deploy requires release; the deploy providers require deploy; the task
// providers require task.
func TestExtensions_EmbeddedRequirements(t *testing.T) {
	exts, err := plugin.Extensions(nerv.ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"task": nil, "task-teamwork": {"task"},
		"release": nil, "release-conventional-commits": {"release"}, "release-docker-compose": {"release"}, "release-bash": {"release"},
		"deploy": {"release"}, "deploy-github": {"deploy"}, "deploy-ssh-compose": {"deploy"},
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

// S1: the suggestion order is task and its providers, then release and
// its providers, then deploy, then the deploy providers; anything else
// after, by name.
func TestSuggestionOrder(t *testing.T) {
	exts, err := plugin.Extensions(nerv.ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range plugin.SuggestionOrder(exts) {
		got = append(got, e.Name)
	}
	want := []string{"task", "task-teamwork", "release", "release-bash", "release-conventional-commits", "release-docker-compose", "deploy", "deploy-github", "deploy-ssh-compose"}
	if !slices.Equal(got, want) {
		t.Errorf("SuggestionOrder = %v, want %v", got, want)
	}
	other := []plugin.Extension{{Name: "zeta"}, {Name: "deploy", Order: 3, Requires: []string{"release"}}, {Name: "alpha"}, {Name: "release", Order: 2}}
	got = nil
	for _, e := range plugin.SuggestionOrder(other) {
		got = append(got, e.Name)
	}
	if want := []string{"release", "deploy", "alpha", "zeta"}; !slices.Equal(got, want) {
		t.Errorf("SuggestionOrder = %v, want %v", got, want)
	}
}

// S4 (L-change): release and deploy bundle every provider they ship
// (installed with them, never asked); task's are suggested.
func TestExtensions_BundledProviders(t *testing.T) {
	exts, err := plugin.Extensions(nerv.ExtensionsFS())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"release": {"release-bash", "release-conventional-commits", "release-docker-compose"},
		"deploy":  {"deploy-github", "deploy-ssh-compose"},
	}
	for _, e := range exts {
		if !slices.Equal(e.Bundle, want[e.Name]) {
			t.Errorf("%s bundle = %v, want %v", e.Name, e.Bundle, want[e.Name])
		}
		if bundled := e.Provider && e.Owner != "task"; e.Bundled != bundled {
			t.Errorf("%s bundled = %v, want %v", e.Name, e.Bundled, bundled)
		}
	}
}
