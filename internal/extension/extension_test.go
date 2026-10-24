package extension_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
)

const acmeExtension = `name: acme
contract_version: 1
suggest_order: 4
requires: [release]
runtime: exec
argv: ["{nerv}", acme]
events: [run.started]
config:
  - key: region
    scope: repo
    type: string
    prompt: The Acme region
provider_fields:
  - name: label
    type: string
    required: true
  - name: tags
    type: list
slots:
  - name: box
    cardinality: per-target
    default: tin
    operations:
      ship: required
      check: optional
`

const tinProvider = `name: tin
extension: acme
slot: box
contract_version: 1
runtime: exec
requires: [tin]
label: Tin box
config:
  - key: size
    scope: repo
    type: int
    default: "3"
    prompt: The box size
operations:
  ship:
    - argv: [tin, ship, "{version}"]
  check: not_implemented
`

const agentProvider = `name: paper
extension: acme
slot: box
contract_version: 1
runtime: agent
label: Paper box
mcp:
  server: paper
  tools: [paper_ship]
operations:
  ship: procedures/ship.md
  check: not_implemented
`

func replace(t *testing.T, text, from, to string) string {
	t.Helper()
	if !strings.Contains(text, from) {
		t.Fatalf("fixture lacks %q", from)
	}
	return strings.Replace(text, from, to, 1)
}

func TestParseManifest_AcceptsAValidExtension(t *testing.T) {
	m, err := extension.ParseManifest([]byte(acmeExtension))
	if err != nil {
		t.Fatal(err)
	}
	slot, ok := m.Slot("box")
	if m.Name != "acme" || m.SuggestOrder != 4 || m.Runtime != extension.RuntimeExec || !ok || slot.Default != "tin" ||
		slot.Operations["ship"] != extension.Required || !m.Subscribes("run.started") {
		t.Errorf("manifest = %+v", m)
	}
	if got := slot.OperationNames(); strings.Join(got, ",") != "check,ship" {
		t.Errorf("OperationNames = %v", got)
	}
}

func TestParseManifest_RefusesInvalidExtensions(t *testing.T) {
	tests := []struct{ name, from, to, want string }{
		{"unknown field", "suggest_order: 4", "suggest_order: 4\nshell: bash", "field shell not found"},
		{"contract", "contract_version: 1", "contract_version: 2", "contract_version 2 is not supported (this nerv understands 1)"},
		{"bad name", "name: acme", "name: Acme", `name "Acme"`},
		{"bad runtime", "runtime: exec", "runtime: shell", `runtime "shell" must be one of agent, exec`},
		{"exec without argv", "argv: [\"{nerv}\", acme]\n", "", "argv is required for runtime exec"},
		{"argv a path", `"{nerv}", acme`, `/bin/acme, run`, `argv[0] "/bin/acme" must be {nerv} or an executable name`},
		{"agent entry", "runtime: exec\nargv: [\"{nerv}\", acme]", "runtime: agent\nagent: dispatcher", `agent "dispatcher" must name the entry agent as <plugin>:<agent>`},
		{"unknown event", "events: [run.started]", "events: [run.exploded]", "events: run.exploded is not a core event"},
		{"self requirement", "requires: [release]", "requires: [acme]", "an extension cannot require itself"},
		{"bad cardinality", "cardinality: per-target", "cardinality: few", `cardinality "few" must be one of one, many, per-target`},
		{"bad level", "check: optional", "check: maybe", `operations.check "maybe" must be required or optional`},
		{"field shadows a common one", "- name: tags", "- name: slot", `"slot" is a common provider.yaml field`},
		{"config prompt", "    prompt: The Acme region\n", "", "config[0].prompt is required"},
		{"negative order", "suggest_order: 4", "suggest_order: -1", "suggest_order -1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extension.ParseManifest([]byte(replace(t, acmeExtension, tt.from, tt.to)))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ParseManifest: %v, want %q", err, tt.want)
			}
		})
	}
}

func TestParseProvider_AcceptsExecAndAgentProviders(t *testing.T) {
	ext, err := extension.ParseManifest([]byte(acmeExtension))
	if err != nil {
		t.Fatal(err)
	}
	p, err := extension.ParseProvider([]byte(tinProvider))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.CheckAgainst(ext); err != nil {
		t.Fatal(err)
	}
	var label string
	if err := p.DecodeField("label", &label); err != nil || label != "Tin box" {
		t.Errorf("label = %q, %v", label, err)
	}
	if !p.Implements("ship") || p.Implements("check") || p.Operations["ship"].Steps[0].Argv[2].Value != "{version}" {
		t.Errorf("operations = %+v", p.Operations)
	}
	a, err := extension.ParseProvider([]byte(agentProvider))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CheckAgainst(ext); err != nil {
		t.Fatal(err)
	}
	if a.Operations["ship"].Procedure != "procedures/ship.md" || a.MCP.Server != "paper" {
		t.Errorf("agent provider = %+v", a)
	}
}

// S6: one provider.yaml shape for every provider, checked against the slot
// its extension declares.
func TestProvider_RefusesInvalidProviders(t *testing.T) {
	ext, err := extension.ParseManifest([]byte(acmeExtension))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, base, from, to, want string }{
		{"unknown common-level field", tinProvider, "label: Tin box", "label: Tin box\nshell: bash", "field shell not found"},
		{"unknown nested field", tinProvider, "    prompt: The box size", "    prompt: The box size\n    hidden: true", "field hidden not found"},
		{"contract", tinProvider, "contract_version: 1", "contract_version: 2", "contract_version 2 is not supported"},
		{"unknown slot", tinProvider, "slot: box", "slot: crate", `slot "crate" is not a slot of extension acme (box)`},
		{"other extension", tinProvider, "extension: acme", "extension: other", `extension "other" is not acme`},
		{"missing required operation", tinProvider, "  ship:\n    - argv: [tin, ship, \"{version}\"]\n", "", "operations.ship: missing"},
		{"required operation not implemented", tinProvider, "  ship:\n    - argv: [tin, ship, \"{version}\"]\n", "  ship: not_implemented\n", "operations.ship: required by slot acme.box, it cannot be not_implemented"},
		{"unknown operation", tinProvider, "  check: not_implemented", "  check: not_implemented\n  burn: not_implemented", "operations.burn is not an operation of slot acme.box"},
		{"missing optional operation", tinProvider, "  check: not_implemented\n", "", "operations.check: missing (implement it or declare not_implemented)"},
		{"required extension field", tinProvider, "label: Tin box\n", "", "label is required by extension acme"},
		{"extension field type", tinProvider, "label: Tin box", "label: Tin box\ntags: none", "tags must be a list"},
		{"exec command not required", tinProvider, "argv: [tin, ship", "argv: [sh, ship", `argv[0] "sh" must be {nerv} or an executable listed in requires`},
		{"exec procedure", tinProvider, "  check: not_implemented", "  check: check.md", `operations.check: an operation is a list of steps or not_implemented, not "check.md"`},
		{"exec mcp", tinProvider, "requires: [tin]", "requires: [tin]\nmcp:\n  server: x\n  tools: [y]", "mcp is for runtime agent, not exec"},
		{"agent requires", agentProvider, "runtime: agent", "runtime: agent\nrequires: [paper]", "requires is for runtime exec"},
		{"agent procedure path", agentProvider, "procedures/ship.md", "../ship.md", `operations.ship: "../ship.md" must be a relative .md path`},
		{"agent tools", agentProvider, "tools: [paper_ship]", "tools: []", "mcp.tools must name every tool"},
		{"bad runtime", tinProvider, "runtime: exec", "runtime: wasm", `runtime "wasm" must be one of agent, exec`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := extension.ParseProvider([]byte(replace(t, tt.base, tt.from, tt.to)))
			if err == nil {
				err = p.CheckAgainst(ext)
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("provider: %v, want %q", err, tt.want)
			}
		})
	}
}

func tree() fstest.MapFS {
	return fstest.MapFS{
		"acme/extension.yaml":              {Data: []byte(acmeExtension)},
		"acme-tin/provider.yaml":           {Data: []byte(tinProvider)},
		"acme-paper/provider.yaml":         {Data: []byte(agentProvider)},
		"acme-paper/procedures/ship.md":    {Data: []byte("# ship\n")},
		"release/extension.yaml":           {Data: []byte("name: release\ncontract_version: 1\nruntime: exec\nargv: [\"{nerv}\", release]\n")},
		"acme-paper/procedures/unused.txt": {Data: []byte("x")},
	}
}

func TestLoadProvider_ChecksTheFolderTheExtensionAndTheFiles(t *testing.T) {
	fsys := tree()
	p, ext, err := extension.LoadProvider(fsys, "acme-tin")
	if err != nil || p.Name != "tin" || ext.Name != "acme" {
		t.Fatalf("LoadProvider = %+v, %+v, %v", p, ext, err)
	}
	if _, _, err := extension.LoadProvider(fsys, "acme-paper"); err != nil {
		t.Fatalf("agent provider: %v", err)
	}

	tests := []struct {
		name string
		edit func(fstest.MapFS)
		dir  string
		want string
	}{
		{"folder name mismatch", func(f fstest.MapFS) { f["acme-box/provider.yaml"] = f["acme-tin/provider.yaml"] }, "acme-box",
			"acme-box/provider.yaml: a provider of extension acme named tin lives in the folder acme-tin"},
		{"extension not available", func(f fstest.MapFS) { delete(f, "acme/extension.yaml") }, "acme-tin",
			`acme-tin/provider.yaml: extension "acme" is not available (no acme/extension.yaml)`},
		{"procedure missing", func(f fstest.MapFS) { delete(f, "acme-paper/procedures/ship.md") }, "acme-paper",
			"operations.ship: procedures/ship.md not found"},
		{"extension invalid", func(f fstest.MapFS) {
			f["acme/extension.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(acmeExtension, "contract_version: 1", "contract_version: 9", 1))}
		}, "acme-tin", "acme/extension.yaml: contract_version 9 is not supported"},
		{"slot misfit", func(f fstest.MapFS) {
			f["acme-tin/provider.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(tinProvider, "slot: box", "slot: crate", 1))}
		}, "acme-tin", `acme-tin/provider.yaml: slot "crate" is not a slot of extension acme`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tree()
			tt.edit(f)
			_, _, err := extension.LoadProvider(f, tt.dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("LoadProvider: %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadManifest_NameMustBeItsFolder(t *testing.T) {
	f := tree()
	f["other/extension.yaml"] = f["acme/extension.yaml"]
	if _, err := extension.LoadManifest(f, "other"); err == nil || !strings.Contains(err.Error(), `name "acme" must match its folder "other"`) {
		t.Errorf("LoadManifest: %v", err)
	}
}

func TestResponse_FailureAndData(t *testing.T) {
	r, ok := extension.ParseResponse([]byte(`  {"status":"needs_input","message":"which project?","data":{"requires":["sshpass"]}}` + "\n"))
	if !ok || !r.Failure() || r.Message != "which project?" {
		t.Fatalf("ParseResponse = %+v, %v", r, ok)
	}
	var data struct{ Requires []string }
	if err := r.DecodeData(&data); err != nil || len(data.Requires) != 1 || data.Requires[0] != "sshpass" {
		t.Errorf("data = %+v, %v", data, err)
	}
	for _, s := range []string{extension.StatusOK, extension.StatusSkipped} {
		if (extension.Response{Status: s}).Failure() {
			t.Errorf("%s is a failure", s)
		}
	}
	if (extension.Response{Status: extension.StatusBlocked}).Failure() != true {
		t.Error("blocked is not a failure")
	}
	if _, ok := extension.ParseResponse([]byte("plain text")); ok {
		t.Error("plain text parsed as a response")
	}
}
