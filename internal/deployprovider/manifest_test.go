package deployprovider

import (
	"strings"
	"testing"
)

const validHost = `name: acme
contract_version: 1
kind: git-host
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
contract_version: 1
kind: target
requires: [docker]
operations:
  publish:
    - argv: [docker, load, -i, "{item}"]
      for_each: image_tars
    - argv: [docker, push, "{item}"]
      for_each: images
  deploy:
    - argv: ["{nerv}", deploy-box, deploy]
  verify: not_implemented
`

func TestParse_AcceptsAValidGitHostAndTarget(t *testing.T) {
	h, err := Parse([]byte(validHost))
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != KindGitHost || !h.Implements(OpCreateRelease) || h.Operations[OpCreateRelease].Steps[0].Argv[5].When != "prerelease" {
		t.Errorf("host = %+v", h)
	}
	tg, err := Parse([]byte(validTarget))
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
		{"unknown field", "kind: git-host", "kind: git-host\nshell: bash", "field shell not found"},
		{"bad kind", "kind: git-host", "kind: registry", `kind "registry" must be one of git-host, target`},
		{"contract", "contract_version: 1", "contract_version: 2", "contract_version 2 is not supported"},
		{"bad name", "name: acme", "name: Acme", `name "Acme"`},
		{"missing op", "  createRelease:\n    - argv: [acme, create, \"{tag}\", --notes-file, \"{notes_file}\", {arg: --prerelease, when: prerelease}]\n", "", "operations.createRelease: missing"},
		{"required op not implemented", "  createRelease:\n    - argv: [acme, create, \"{tag}\", --notes-file, \"{notes_file}\", {arg: --prerelease, when: prerelease}]\n", "  createRelease: not_implemented\n", "operations.createRelease: required"},
		{"foreign op", "  releaseExists:", "  deploy:\n    - argv: [acme]\n  releaseExists:", "operations.deploy is not an operation of kind git-host"},
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
			_, err := Parse([]byte(strings.Replace(validHost, tt.from, tt.to, 1)))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Parse: %v, want %q", err, tt.want)
			}
		})
	}
	bad := strings.Replace(validTarget, "verify: not_implemented", "verify: maybe", 1)
	if _, err := Parse([]byte(bad)); err == nil || !strings.Contains(err.Error(), "list of steps or not_implemented") {
		t.Errorf("verify: maybe: %v", err)
	}
	bad = strings.Replace(validTarget, "      for_each: images", "      for_each: files", 1)
	if _, err := Parse([]byte(bad)); err == nil || !strings.Contains(err.Error(), `for_each "files"`) {
		t.Errorf("for_each files: %v", err)
	}
	bad = strings.Replace(validTarget, "  deploy:\n    - argv: [\"{nerv}\", deploy-box, deploy]\n", "  deploy: not_implemented\n", 1)
	if _, err := Parse([]byte(bad)); err == nil || !strings.Contains(err.Error(), "operations.deploy: required") {
		t.Errorf("deploy not implemented: %v", err)
	}
}
