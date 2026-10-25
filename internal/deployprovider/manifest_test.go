package deployprovider

import (
	"strings"
	"testing"

	nerv "github.com/war-apps/nerv-by-gentle-ai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
)

const validHost = `name: acme
extension: deploy
slot: host
contract_version: 1
runtime: exec
requires: [acme]
config:
  - key: repo
    scope: repo
    type: string
    prompt: The repository
operations:
  releaseExists:
    - argv: [acme, view, "{tag}", --repo, "{config.repo}"]
      absent_stderr: not found
  createRelease:
    - argv: [acme, create, "{tag}", --notes-file, "{notes_file}", {arg: --prerelease, when: prerelease}]
`

const validTarget = `name: box
extension: deploy
slot: target
contract_version: 1
runtime: exec
requires: [docker]
operations:
  preflight: not_implemented
  publish:
    - argv: [docker, load, -i, "{item}"]
      for_each: image_tars
    - argv: [docker, push, "{item}"]
      for_each: images
  deploy:
    - argv: ["{nerv}", deploy-box, deploy]
  verify: not_implemented
`

// deployExtension is the deploy extension's manifest nerv ships.
func deployExtension(t *testing.T) *extension.Manifest {
	t.Helper()
	m, err := extension.LoadManifest(nerv.ExtensionsFS(), Extension)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func parse(t *testing.T, text string) (*Manifest, error) {
	t.Helper()
	return Parse(deployExtension(t), []byte(text))
}

func TestParse_AcceptsAValidGitHostAndTarget(t *testing.T) {
	h, err := parse(t, validHost)
	if err != nil {
		t.Fatal(err)
	}
	if h.Slot != SlotHost || !h.Implements(OpCreateRelease) || h.Operations[OpCreateRelease].Steps[0].Argv[5].When != "prerelease" {
		t.Errorf("host = %+v", h)
	}
	tg, err := parse(t, validTarget)
	if err != nil {
		t.Fatal(err)
	}
	if tg.Implements(OpVerify) || !tg.Implements(OpPublish) || tg.Operations[OpPublish].Steps[1].ForEach != "images" {
		t.Errorf("target = %+v", tg)
	}
}

func TestParse_RefusesInvalidManifests(t *testing.T) {
	tests := []struct {
		name, from, to, want string
	}{
		{"unknown field", "slot: host", "slot: host\nshell: bash", "field shell not found"},
		{"unknown slot", "slot: host", "slot: registry", `slot "registry" is not a slot of extension deploy (host, target)`},
		{"agent runtime", "runtime: exec\nrequires: [acme]", "runtime: agent\nmcp:\n  server: acme\n  tools: [acme_view]", "operations.releaseExists: an agent provider names a procedure .md file"},
		{"map config", "type: string", "type: map", `config[0].type "map" must be one of string, int, bool`},
		{"contract", "contract_version: 1", "contract_version: 2", "contract_version 2 is not supported"},
		{"bad name", "name: acme", "name: Acme", `name "Acme"`},
		{"missing op", "  createRelease:\n    - argv: [acme, create, \"{tag}\", --notes-file, \"{notes_file}\", {arg: --prerelease, when: prerelease}]\n", "", "operations.createRelease: missing"},
		{"required op not implemented", "  createRelease:\n    - argv: [acme, create, \"{tag}\", --notes-file, \"{notes_file}\", {arg: --prerelease, when: prerelease}]\n", "  createRelease: not_implemented\n", "operations.createRelease: required by slot deploy.host"},
		{"foreign op", "  releaseExists:", "  deploy:\n    - argv: [acme]\n  releaseExists:", "operations.deploy is not an operation of slot deploy.host"},
		{"partial variable", `"{notes_file}"`, `"--notes={notes_file}"`, "a variable must be the whole argument"},
		{"unknown variable", `"{notes_file}"`, `"{notes}"`, `"{notes}": unknown variable`},
		{"undeclared config", `"{config.repo}"`, `"{config.owner}"`, "config key owner is not declared"},
		{"item without for_each", `"{tag}", --repo`, `"{item}", --repo`, "{item} needs for_each"},
		{"command not required", "argv: [acme, create", "argv: [sh, create", `argv[0] "sh" must be {nerv} or an executable listed in requires`},
		{"command a path", "requires: [acme]", "requires: [acme, /bin/sh]", `"/bin/sh" must be an executable name`},
		{"bad when", "when: prerelease}", "when: draft}", `when "draft" must be one of prerelease`},
		{"bad regexp", "absent_stderr: not found", "absent_stderr: \"(\"", "absent_stderr: error parsing regexp"},
		{"bad scope", "scope: repo", "scope: global", `scope "global"`},
		{"missing prompt", "    prompt: The repository\n", "", "config[0].prompt is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(validHost, tt.from) {
				t.Fatalf("fixture lacks %q", tt.from)
			}
			_, err := parse(t, strings.Replace(validHost, tt.from, tt.to, 1))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Parse: %v, want %q", err, tt.want)
			}
		})
	}
	bad := strings.Replace(validTarget, "verify: not_implemented", "verify: maybe", 1)
	if _, err := parse(t, bad); err == nil || !strings.Contains(err.Error(), "list of steps or not_implemented") {
		t.Errorf("verify: maybe: %v", err)
	}
	bad = strings.Replace(validTarget, "      for_each: images", "      for_each: files", 1)
	if _, err := parse(t, bad); err == nil || !strings.Contains(err.Error(), `for_each "files"`) {
		t.Errorf("for_each files: %v", err)
	}
	bad = strings.Replace(validTarget, "  deploy:\n    - argv: [\"{nerv}\", deploy-box, deploy]\n", "  deploy: not_implemented\n", 1)
	if _, err := parse(t, bad); err == nil || !strings.Contains(err.Error(), "operations.deploy: required") {
		t.Errorf("deploy not implemented: %v", err)
	}
}

// A preflight runs once, before anything is written: no for_each.
func TestParse_PreflightRunsOnce(t *testing.T) {
	bad := strings.Replace(validTarget, "  preflight: not_implemented", "  preflight:\n    - argv: [docker, info]\n      for_each: images", 1)
	if _, err := parse(t, bad); err == nil || !strings.Contains(err.Error(), "for_each: preflight runs once") {
		t.Errorf("preflight for_each: %v", err)
	}
}

// The artifact's build provider is {build_provider} (the artifact field
// since the release providers); {build_mode} is gone.
func TestParse_BuildProviderVariable(t *testing.T) {
	ok := strings.Replace(validTarget, `["{nerv}", deploy-box, deploy]`, `["{nerv}", deploy-box, deploy, "{build_provider}"]`, 1)
	if _, err := parse(t, ok); err != nil {
		t.Errorf("{build_provider}: %v", err)
	}
	old := strings.Replace(validTarget, `["{nerv}", deploy-box, deploy]`, `["{nerv}", deploy-box, deploy, "{build_mode}"]`, 1)
	if _, err := parse(t, old); err == nil || !strings.Contains(err.Error(), `"{build_mode}": unknown variable`) {
		t.Errorf("{build_mode}: %v", err)
	}
}
