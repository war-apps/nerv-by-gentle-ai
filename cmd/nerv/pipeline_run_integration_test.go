//go:build semrel_integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

// passingSteps runs nothing: every dotnet command of validate and test
// succeeds (dotnet is faked at the process boundary).
type passingSteps struct{ ran *[]string }

func (p passingSteps) Run(_ context.Context, s shiprun.Step, _ shiprun.Vars) error {
	*p.ran = append(*p.ran, strings.Join(s.Exec[:2], " "))
	return nil
}

// TestIntegration_PipelineRunDryRunGetsThroughEveryStep: the dry run
// /nerv:pipeline always starts with, through the real step implementations
// (real git, a local bare origin, semantic-release's real dry run with the
// built-in configuration), must pass every step for a releasable
// repository and for one with nothing to release. Faked at the process
// boundary: dotnet (validate, test), docker (the image build) and the
// deploy's ssh/infisical runner, which must not be called at all.
func TestIntegration_PipelineRunDryRunGetsThroughEveryStep(t *testing.T) {
	requireShipToolchain(t)
	cache := t.TempDir()
	for _, tt := range []struct {
		name    string
		feature bool
	}{
		{"releasable", true},
		{"nothing to release", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			shipEnv(t)
			_, work := newShipRepos(t)
			work.commit("chore: init", "a.txt")
			work.git("tag", "v1.0.0")
			work.git("branch", "develop")
			if tt.feature {
				work.commit("feat: add gadgets", "b.txt")
			}
			work.git("push", "-q", "origin", "main", "develop", "--tags")
			work.write(".ai-workflow.yml", `pipeline:
  dotnet: { solution: App.slnx }
  images: [{ name: ghcr.io/acme/app }]
  deploy:
    environments: [production]
`)
			for k, v := range map[string]string{"DOCKER_SSH_HOST": "10.0.0.1", "DOCKER_SSH_USER": "deploy", "DOCKER_SSH_PASS": "pw",
				"DOCKER_COMPOSE_PATH": "/srv", "DOCKER_COMPOSE_SCRIPT": "./manage.sh", "DOCKER_CONTAINER_NAME": "app",
				"DOCKER_CONTAINER_VARIANT": "green", "DOCKER_IMAGE_VERSION": "latest"} {
				t.Setenv("NERV_DEPLOY_PRODUCTION_"+k, v)
			}

			var dotnet []string
			opts := shipOptions(t, cache)
			opts.PipelineSteps = func(string, io.Writer) pipeline.StepRunner { return passingSteps{&dotnet} }
			docker := &recProc{}
			opts.DockerProc = docker
			deployProc := &recProc{}
			opts.DeployProc = deployProc
			var stdout, stderr bytes.Buffer
			code := run([]string{"pipeline", "run", "--cwd", work.dir, "--dry-run", "--json"}, &stdout, &stderr, opts)
			var r pipeline.RunResult
			if err := json.Unmarshal(stdout.Bytes(), &r); err != nil {
				t.Fatalf("%v\nstdout %s\nstderr %s", err, stdout.String(), stderr.String())
			}
			if code != 0 || r.Status != "passed" || !slices.Equal(outcomes(r), []string{"validate:passed:0", "test:passed:0", "release:passed:0", "docker:passed:0", "sync:passed:0", "deploy:passed:0"}) {
				t.Fatalf("exit %d %q error %q\n%s", code, outcomes(r), r.Error, stderr.String())
			}
			if len(deployProc.cmds) != 0 {
				t.Errorf("the dry run ran %d deploy commands", len(deployProc.cmds))
			}
			rel, err := pipeline.ReadReleaseOutputs(work.dir)
			if err != nil || !rel.DryRun || rel.Published != tt.feature {
				t.Fatalf("release.json %+v %v", rel, err)
			}
			dockerRes, err := pipeline.ReadDockerResult(work.dir)
			if err != nil || dockerRes.Status != "passed" {
				t.Errorf("docker.json %+v %v", dockerRes, err)
			}
			sync, _ := readSyncJSON(t, work.dir)
			_, deploy, _ := readDeployJSON(t, work.dir)
			if tt.feature {
				if rel.Version != "1.1.0" || !dockerRes.DryRun || len(docker.sub("build")) != 1 || len(docker.sub("push")) != 0 {
					t.Errorf("release %s, docker dry run %v, builds %d pushes %d", rel.Version, dockerRes.DryRun, len(docker.sub("build")), len(docker.sub("push")))
				}
				if len(sync.Branches) != 1 || sync.Branches[0].Branch != "develop" || sync.Branches[0].Status != "would-merge" {
					t.Errorf("sync.json %+v", sync)
				}
				if !deploy.DryRun || len(deploy.Environments) != 1 || deploy.Environments[0].Status != "would-deploy" {
					t.Errorf("deploy.json %+v", deploy)
				}
			} else {
				if len(docker.cmds) != 0 || len(sync.Branches) != 0 || len(deploy.Environments) != 0 {
					t.Errorf("nothing to release, yet docker %d commands, sync %+v, deploy %+v", len(docker.cmds), sync, deploy)
				}
			}
			for _, cmd := range []string{"dotnet restore", "dotnet format", "dotnet build", "dotnet test"} {
				if !slices.Contains(dotnet, cmd) {
					t.Errorf("%s not run: %q", cmd, dotnet)
				}
			}

			// A real run on these dry-run records still refuses: run sync on
			// its own without --dry-run.
			var out2, err2 bytes.Buffer
			if code := run([]string{"pipeline", "sync", "--cwd", work.dir, "--yes", "--json"}, &out2, &err2, opts); tt.feature && (code != 1 || !strings.Contains(err2.String(), "dry run")) {
				t.Errorf("real sync on dry-run records: exit %d\n%s", code, err2.String())
			}
		})
	}
}
