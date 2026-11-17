package ecosystem

import (
	"slices"
	"testing"
)

func TestSelected_NoToolsSkipsEverything(t *testing.T) {
	calls := 0
	confirm := func(Tool, State) bool { calls++; return true }
	got, err := Selected(Catalogue(), OfferOptions{NoTools: true}, confirm, nil)
	if err != nil {
		t.Fatalf("Selected() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Selected() = %v, want none", got)
	}
	if calls != 0 {
		t.Errorf("confirm was asked %d times, want 0 (--no-tools)", calls)
	}
}

func TestSelected_NamedToolsSkipTheConfirmPrompt(t *testing.T) {
	calls := 0
	confirm := func(Tool, State) bool { calls++; return true }
	got, err := Selected(Catalogue(), OfferOptions{Tools: []string{"nerv-task"}}, confirm, nil)
	if err != nil {
		t.Fatalf("Selected() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "nerv-task" {
		t.Errorf("Selected() = %v, want [nerv-task]", got)
	}
	if calls != 0 {
		t.Errorf("confirm was asked %d times, want 0 (explicit --tools)", calls)
	}
}

func TestSelected_UnknownNamedToolIsRefused(t *testing.T) {
	_, err := Selected(Catalogue(), OfferOptions{Tools: []string{"not-a-tool"}}, nil, nil)
	if err == nil {
		t.Fatal("Selected() error = nil, want a refusal for an unknown tool name")
	}
}

func TestSelected_NilConfirmSelectsNothing(t *testing.T) {
	got, err := Selected(Catalogue(), OfferOptions{}, nil, nil)
	if err != nil {
		t.Fatalf("Selected() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Selected() = %v, want none (no terminal, no flags)", got)
	}
}

func TestSelected_AsksConfirmPerToolInCatalogueOrder(t *testing.T) {
	tools := []Tool{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	var asked []string
	confirm := func(t Tool, _ State) bool {
		asked = append(asked, t.Name)
		return t.Name == "b"
	}
	states := map[string]State{"b": {Installed: true}}
	got, err := Selected(tools, OfferOptions{}, confirm, states)
	if err != nil {
		t.Fatalf("Selected() error = %v", err)
	}
	if !slices.Equal(asked, []string{"a", "b", "c"}) {
		t.Errorf("asked = %v, want [a b c] in catalogue order", asked)
	}
	if len(got) != 1 || got[0].Name != "b" {
		t.Errorf("Selected() = %v, want [b]", got)
	}
}

func TestRenderState_Variants(t *testing.T) {
	cases := []struct {
		name  string
		state State
		want  string
	}{
		{"not installed", State{}, "not installed"},
		{"version unknown", State{Installed: true, VersionUnknown: true}, "installed, version unknown"},
		{"installed no latest", State{Installed: true, Version: "1.0.0"}, "installed (1.0.0)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RenderState(c.state, "")
			if got != c.want {
				t.Errorf("RenderState() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRenderState_OutdatedAgainstLatest(t *testing.T) {
	got := RenderState(State{Installed: true, Version: "1.0.0"}, "v1.1.0")
	want := "installed (1.0.0), outdated (latest v1.1.0)"
	if got != want {
		t.Errorf("RenderState() = %q, want %q", got, want)
	}
}

func TestRenderState_UpToDateAgainstLatest(t *testing.T) {
	got := RenderState(State{Installed: true, Version: "1.1.0"}, "v1.1.0")
	want := "installed (1.1.0), up to date"
	if got != want {
		t.Errorf("RenderState() = %q, want %q", got, want)
	}
}
