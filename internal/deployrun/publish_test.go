package deployrun

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

func boolPtr(b bool) *bool { return &b }

// stepNames are the report's step names, in order.
func stepNames(r Report) []string {
	var names []string
	for _, s := range r.Steps {
		names = append(names, s.Name)
	}
	return names
}

// With no target, a deploy is the release alone: it pushes the release,
// loads every image tar once, pushes every manifest image once, creates
// the GitHub release, and runs nothing against a server.
func TestRun_PublishesTheImagesWithoutTargets(t *testing.T) {
	e := newEnv(t)
	e.cfg.Targets = nil
	e.writeArtifact([]string{"reg.example/app:1.0.0-alpha.1", "reg.example/app:alpha", "reg.example/app:latest"}, "app.tar", "worker.tar")
	rep, err := e.run(e.options())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := stepNames(rep), []string{"verify-artifact", "clone", "commit", "tag", "notes", "push", "publish", "release"}; !slices.Equal(got, want) {
		t.Errorf("steps %v, want %v", got, want)
	}
	wantStatuses(t, rep, map[string]string{"push": StatusDone, "publish": StatusDone, "release": StatusDone})
	outputs := filepath.Join(e.dir, "outputs")
	want := []string{
		"docker [load] [-i] [" + filepath.Join(outputs, "app.tar") + "]",
		"docker [load] [-i] [" + filepath.Join(outputs, "worker.tar") + "]",
		"docker [push] [reg.example/app:1.0.0-alpha.1]",
		"docker [push] [reg.example/app:alpha]",
		"docker [push] [reg.example/app:latest]",
		"gh [release] [view] [" + tag + "]",
		"gh [release] [create] [" + tag + "] [--title] [" + tag + "] [--notes-file] [" + filepath.Join(e.dir, "notes.md") + "] [--prerelease] [--verify-tag]",
	}
	if calls := e.log(); !slices.Equal(calls, want) {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
	if d := rep.Step("publish").Detail; !strings.Contains(d, "reg.example/app:latest") {
		t.Errorf("publish detail %q does not name the pushed images", d)
	}
}

// An artifact without images has nothing to publish: the step is skipped,
// docker is not needed, and the release is still created.
func TestRun_SkipsThePublishWithoutImages(t *testing.T) {
	e := newEnv(t)
	e.cfg.Targets = nil
	e.writeArtifact(nil)
	opts := e.options()
	opts.LookPath = func(name string) (string, error) {
		if name == "docker" {
			return "", exec.ErrNotFound
		}
		return exec.LookPath(name)
	}
	rep, err := e.run(opts)
	if err != nil {
		t.Fatal(err)
	}
	wantStatuses(t, rep, map[string]string{"publish": StatusSkipped, "release": StatusDone})
	for _, c := range e.log() {
		if strings.HasPrefix(c, "docker") {
			t.Errorf("docker ran without images: %s", c)
		}
	}
}

// With images, docker on PATH and the registry's credentials are checked
// before anything is written; the message says what to run.
func TestRun_PublishPreflightRefusesBeforeAnyGitWrite(t *testing.T) {
	t.Run("docker missing", func(t *testing.T) {
		e := newEnv(t)
		e.cfg.Targets = nil
		refs := e.originRefs()
		opts := e.options()
		opts.LookPath = func(name string) (string, error) {
			if name == "docker" {
				return "", exec.ErrNotFound
			}
			return exec.LookPath(name)
		}
		rep, err := e.run(opts)
		if !refusal.Is(err) || !strings.Contains(err.Error(), "docker is not on PATH (deploy.publish needs it)") {
			t.Fatalf("Run: %v", err)
		}
		wantStatuses(t, rep, map[string]string{"clone": StatusNotRun, "push": StatusNotRun, "publish": StatusNotRun})
		if e.originRefs() != refs {
			t.Errorf("a refused deploy changed origin")
		}
	})
	for _, tt := range []struct{ name, config string }{
		{"no docker config", ""},
		{"other registry", `{"auths":{"ghcr.io":{"auth":"eDp5"}}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			if tt.config == "" {
				t.Setenv("DOCKER_CONFIG", t.TempDir())
			} else {
				e.dockerConfig(tt.config)
			}
			refs := e.originRefs()
			rep, err := e.run(e.options())
			if !refusal.Is(err) || !strings.Contains(err.Error(), "no credentials for reg.example in the docker config: run docker login reg.example") {
				t.Fatalf("Run: %v", err)
			}
			wantStatuses(t, rep, map[string]string{"clone": StatusNotRun, "push": StatusNotRun})
			if e.originRefs() != refs || e.effects()[0] != "" {
				t.Errorf("a refused deploy wrote something: %v", e.log())
			}
		})
	}
}

// Credentials come from auths (any spelling of the registry), credHelpers,
// Docker Hub's https://index.docker.io/v1/ entry, and a global credsStore
// (which may hold any registry's, as an ECR helper does without a login).
func TestRegistryCredentials(t *testing.T) {
	for _, tt := range []struct{ image, config string }{
		{"reg.example/app:1", `{"auths":{"https://reg.example":{}}}`},
		{"reg.example:5000/app:1", `{"auths":{"reg.example:5000":{}}}`},
		{"ghcr.io/org/app:1", `{"credHelpers":{"ghcr.io":"gh"}}`},
		{"org/app:1", `{"auths":{"https://index.docker.io/v1/":{}}}`},
		{"localhost/app:1", `{"auths":{"localhost":{}}}`},
		{"123.dkr.ecr.us-east-1.amazonaws.com/app:1", `{"credsStore":"ecr-login"}`},
	} {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "config.json"), tt.config, 0o600)
		if missing := missingCredentials([]string{tt.image}, []string{"DOCKER_CONFIG=" + dir}); len(missing) != 0 {
			t.Errorf("%s with %s: missing %v", tt.image, tt.config, missing)
		}
	}
}

// publish: false leaves publishing to the targets, as before the core step.
func TestRun_PublishFalseLeavesItToTheTargets(t *testing.T) {
	e := newEnv(t)
	e.cfg.Publish = boolPtr(false)
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	rep, err := e.run(e.options())
	if err != nil {
		t.Fatal(err)
	}
	wantStatuses(t, rep, map[string]string{"publish": StatusSkipped, "publish:staging": StatusDone, "release": StatusDone})
	pushes := 0
	for _, c := range e.log() {
		if strings.HasPrefix(c, "docker [push]") {
			pushes++
		}
	}
	if pushes != 1 {
		t.Errorf("docker push ran %d times, want once (the target's)", pushes)
	}
}

// A failing server target leaves the release created: it comes right after
// the push and the publish.
func TestRun_FailingTargetStillLeavesTheRelease(t *testing.T) {
	e := newEnv(t)
	writeFile(t, filepath.Join(e.fake, "fail-deploy"), "", 0o644)
	rep, err := e.run(e.options())
	if err == nil || !strings.Contains(err.Error(), "deploy broke") {
		t.Fatalf("Run: %v, want the deploy failure", err)
	}
	wantStatuses(t, rep, map[string]string{
		"push": StatusDone, "publish": StatusDone, "release": StatusDone,
		"publish:staging": StatusDone, "deploy:staging": StatusFailed, "verify:staging": StatusNotRun,
	})
	if _, err := os.Stat(filepath.Join(e.fake, "release-"+tag)); err != nil {
		t.Errorf("the GitHub release was not created: %v", err)
	}

	// The re-run resumes: origin and the release are kept, the images are
	// pushed again (a no-op for the registry) and the target runs.
	refs := e.originRefs()
	os.Remove(filepath.Join(e.fake, "fail-deploy"))
	rep, err = e.run(e.options())
	if err != nil {
		t.Fatal(err)
	}
	wantStatuses(t, rep, map[string]string{
		"push": StatusSkipped, "publish": StatusDone, "release": StatusSkipped,
		"deploy:staging": StatusDone, "verify:staging": StatusDone,
	})
	if e.originRefs() != refs {
		t.Errorf("the resume changed origin's refs")
	}
}
