package deployrun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/deployconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

// deployScript records what a release-script script receives.
const deployScript = `cat > "$FAKE_DIR/script.stdin"
env | grep '^NERV_DEPLOY_' | sort > "$FAKE_DIR/script.env"
echo '{"status":"ok","message":"deployed the docs"}'
`

func bashTarget(e *world, config map[string]string) {
	e.cfg.Targets = []deployconfig.Target{{Name: "docs", Provider: "script", Config: config}}
}

// S5: release-script deploys a target with the repository's own script: the
// request on its stdin, the deploy variables in its environment; publish
// without a script is skipped and verify runs its own.
func TestRun_DeploysWithTheBashTarget(t *testing.T) {
	e := newEnv(t, map[string]string{"deploy/deploy.sh": deployScript, "deploy/check.sh": `echo '{"status":"ok","message":"docs are up"}'` + "\n"})
	bashTarget(e, map[string]string{"script": "deploy/deploy.sh", "verify_script": "deploy/check.sh"})
	rep, err := e.run(e.options())
	if err != nil {
		t.Fatal(err)
	}
	wantStatuses(t, rep, map[string]string{"push": StatusDone, "publish:docs": StatusSkipped, "deploy:docs": StatusDone, "verify:docs": StatusDone, "release": StatusDone})
	for _, s := range rep.Steps {
		if s.Name == "deploy:docs" && !strings.Contains(s.Detail, "deployed the docs") {
			t.Errorf("deploy:docs detail %q", s.Detail)
		}
	}
	head := git(t, e.origin, "rev-parse", "refs/heads/develop")
	env, _ := os.ReadFile(filepath.Join(e.fake, "script.env"))
	for _, v := range []string{"NERV_DEPLOY_TARGET=docs", "NERV_DEPLOY_VERSION=1.0.0-alpha.1", "NERV_DEPLOY_TAG=" + tag, "NERV_DEPLOY_CHANNEL=alpha",
		"NERV_DEPLOY_PRERELEASE=true", "NERV_DEPLOY_BUILD_PROVIDER=docker-compose", "NERV_DEPLOY_COMMIT=" + head, "NERV_DEPLOY_ARTIFACT_DIR=" + e.dir,
		"NERV_DEPLOY_IMAGES=reg.example/app:1.0.0-alpha.1"} {
		if !strings.Contains(string(env), v+"\n") {
			t.Errorf("script environment lacks %s:\n%s", v, env)
		}
	}
	var req extension.Request
	stdin, _ := os.ReadFile(filepath.Join(e.fake, "script.stdin"))
	if err := json.Unmarshal(stdin, &req); err != nil || req.Operation != "deploy" || req.Provider != "script" || req.Config["script"] != "deploy/deploy.sh" {
		t.Errorf("script stdin %s: %v", stdin, err)
	}
}

// A script answering failed fails its step and stops the deploy there; the
// release, created before the targets, stays.
func TestRun_BashScriptFailureStopsTheDeploy(t *testing.T) {
	e := newEnv(t, map[string]string{"deploy/deploy.sh": `echo '{"status":"failed","message":"the docs host is down"}'` + "\n"})
	bashTarget(e, map[string]string{"script": "deploy/deploy.sh"})
	rep, err := e.run(e.options())
	if err == nil || refusal.Is(err) || !strings.Contains(err.Error(), "the docs host is down") {
		t.Fatalf("Run: %v, want the script's failure", err)
	}
	wantStatuses(t, rep, map[string]string{"push": StatusDone, "deploy:docs": StatusFailed, "verify:docs": StatusNotRun, "release": StatusDone})
}

// release-script's preflight refuses a missing or escaping script before any
// git write.
func TestRun_BashPreflightRefusesBeforeAnyGitWrite(t *testing.T) {
	for script, want := range map[string]string{"deploy/missing.sh": "no such file in the repository", "../outside.sh": "must be a relative path inside the repository"} {
		t.Run(script, func(t *testing.T) {
			e := newEnv(t)
			bashTarget(e, map[string]string{"script": script})
			before := e.originRefs()
			rep, err := e.run(e.options())
			if !refusal.Is(err) || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "docs") {
				t.Fatalf("Run: %v, want a refusal naming the target and %q", err, want)
			}
			if e.originRefs() != before || statuses(rep)["clone"] != StatusNotRun {
				t.Errorf("the refused deploy wrote: clone %q", statuses(rep)["clone"])
			}
		})
	}
}

// The deploy script target was bash in the deploy extension: a target that
// still names it is refused before any git write, with what to do, instead
// of running the release extension's bash build provider.
func TestRun_BashTargetIsRefusedWithItsNewName(t *testing.T) {
	e := newEnv(t)
	e.cfg.Targets = []deployconfig.Target{{Name: "docs", Provider: "bash", Config: map[string]string{"script": "deploy/deploy.sh"}}}
	before := e.originRefs()
	rep, err := e.run(e.options())
	if !refusal.Is(err) || !strings.Contains(err.Error(), "deploy.targets[0] (docs): the bash deploy target is now named script (release-script): set provider: script, or run nerv migrate (nothing was written)") {
		t.Fatalf("Run: %v, want the renamed-target refusal", err)
	}
	if e.originRefs() != before || statuses(rep)["clone"] != StatusNotRun {
		t.Errorf("the refused deploy wrote: clone %q", statuses(rep)["clone"])
	}
}

// The steps' {build_provider} is the artifact's build provider.
func TestEnv_BuildProviderVariable(t *testing.T) {
	d := &deployer{o: Options{Nerv: "/bin/nerv", ArtifactDir: "/a"}, m: &artifact.Manifest{BuildProvider: "docker-compose"}}
	env, err := d.env(provider{})
	if err != nil {
		t.Fatal(err)
	}
	if env.Values["build_provider"] != "docker-compose" {
		t.Errorf("build_provider = %q", env.Values["build_provider"])
	}
	if _, ok := env.Values["build_mode"]; ok {
		t.Error("build_mode is still a variable")
	}
}
