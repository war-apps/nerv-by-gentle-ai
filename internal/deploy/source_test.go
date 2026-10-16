package deploy

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// fakeProc answers each command by its first argument (the subcommand or,
// for ssh, the remote command) and records them.
type fakeProc struct {
	cmds   []proc.Command
	answer func(proc.Command) (proc.Result, error)
	// answerCtx, when set, answers instead with the command's context.
	answerCtx func(context.Context, proc.Command) (proc.Result, error)
}

func (f *fakeProc) Run(ctx context.Context, c proc.Command) (proc.Result, error) {
	f.cmds = append(f.cmds, c)
	if f.answerCtx != nil {
		return f.answerCtx(ctx, c)
	}
	if f.answer == nil {
		return proc.Result{}, nil
	}
	return f.answer(c)
}

func TestEnvSource_ReadsTheNamespacedVariables(t *testing.T) {
	vars := map[string]string{
		"NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_SSH_HOST":          "10.0.0.10",
		"NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_SSH_PASS":          "s3cr3t",
		"NERV_DEPLOY_PRODUCTION_BLUE_DOCKER_CONTAINER_VARIANT": "blue",
		"NERV_DEPLOY_STAGING_DOCKER_SSH_HOST":                  "10.0.0.20",
		"NERV_DEPLOY_PRODUCTION_BLUE_UNRELATED":                "x",
	}
	d, err := EnvSource{Getenv: func(k string) string { return vars[k] }}.Read(context.Background(), "production-blue")
	if err != nil {
		t.Fatal(err)
	}
	if d["DOCKER_SSH_HOST"] != "10.0.0.10" || d["DOCKER_SSH_PASS"] != "s3cr3t" || d["DOCKER_CONTAINER_VARIANT"] != "blue" || len(d) != 3 {
		t.Errorf("data %v", d)
	}
}

func TestInfisicalSource_ReadsTheRootAndTheAppFolder(t *testing.T) {
	f := &fakeProc{answer: func(c proc.Command) (proc.Result, error) {
		path := c.Args[slices.Index(c.Args, "--path")+1]
		switch path {
		case "/":
			return proc.Result{Stdout: []byte(`[{"key":"DOCKER_SSH_HOST","value":"10.2.0.126"},{"key":"DOCKER_SSH_PASS","value":"pw"},{"key":"DOCKER_IMAGE","value":"ignored at the root"}]`)}, nil
		case "/comprobantes-api":
			return proc.Result{Stdout: []byte(`[{"key":"DOCKER_CONTAINER_NAME","value":"comprobantes-api"},{"key":"DOCKER_IMAGE_VERSION","value":"alpha"},{"key":"DOCKER_SSH_HOST","value":"ignored in the app"}]`)}, nil
		}
		return proc.Result{}, errors.New("unexpected path")
	}}
	src := InfisicalSource{Proc: f, ProjectID: "p-1", AppPath: "/comprobantes-api", Domain: "https://infisical.example", Env: []string{"HOME=/h"}}
	d, err := src.Read(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	if d["DOCKER_SSH_HOST"] != "10.2.0.126" || d["DOCKER_SSH_PASS"] != "pw" || d["DOCKER_CONTAINER_NAME"] != "comprobantes-api" || d["DOCKER_IMAGE_VERSION"] != "alpha" || d["DOCKER_IMAGE"] != "" {
		t.Errorf("data %v", d)
	}
	want := []string{"export", "--silent", "--env", "development", "--path", "/", "--projectId", "p-1", "--format", "json",
		"--include-imports=false", "--expand=false", "--secret-overriding=false", "--domain", "https://infisical.example"}
	if len(f.cmds) != 2 || f.cmds[0].Name != "infisical" || !slices.Equal(f.cmds[0].Args, want) || !slices.Equal(f.cmds[0].Env, []string{"HOME=/h"}) {
		t.Errorf("commands %+v", f.cmds)
	}
}

func TestInfisicalSource_FailureNamesTheFolderWithoutOutput(t *testing.T) {
	f := &fakeProc{answer: func(c proc.Command) (proc.Result, error) {
		return proc.Result{Stdout: []byte(`[{"key":"DOCKER_SSH_PASS","value":"leak"}]`), Stderr: []byte("error: environment not found")}, errors.New("exit status 1")
	}}
	_, err := InfisicalSource{Proc: f, ProjectID: "p", AppPath: "/app"}.Read(context.Background(), "staging")
	if err == nil || !strings.Contains(err.Error(), "staging") || !strings.Contains(err.Error(), "environment not found") || strings.Contains(err.Error(), "leak") {
		t.Errorf("err = %v", err)
	}
	if slices.Contains(f.cmds[0].Args, "--domain") {
		t.Error("no --domain without infisical.domain: the CLI session's own")
	}
}

func dataFor(env string) Data {
	return Data{
		"DOCKER_SSH_HOST": "10.0.0.1", "DOCKER_SSH_USER": "deploy", "DOCKER_SSH_PASS": "pw",
		"DOCKER_COMPOSE_PATH": "/srv/apps", "DOCKER_COMPOSE_SCRIPT": "./manage.sh",
		"DOCKER_CONTAINER_NAME": "api", "DOCKER_CONTAINER_VARIANT": env, "DOCKER_IMAGE_VERSION": "alpha",
		"DOCKER_DEPLOY_STRATEGY": "rolling",
	}
}

func TestNewTarget(t *testing.T) {
	tg, err := NewTarget("development", dataFor("alpha"), "password")
	if err != nil || tg.Container != "api-alpha" || tg.Host != "10.0.0.1" || tg.Password != "pw" || !tg.Enabled || tg.ImageVersion != "alpha" {
		t.Fatalf("target %+v, %v", tg, err)
	}
	for _, off := range []string{"false", "0", "No", "OFF"} {
		d := dataFor("alpha")
		d["DOCKER_DEPLOY_ENABLED"] = off
		if tg, _ := NewTarget("development", d, "password"); tg.Enabled {
			t.Errorf("DOCKER_DEPLOY_ENABLED=%s enabled", off)
		}
	}
	d := dataFor("alpha")
	delete(d, "DOCKER_SSH_HOST")
	delete(d, "DOCKER_CONTAINER_NAME")
	delete(d, "DOCKER_SSH_PASS")
	_, err = NewTarget("staging", d, "password")
	for _, want := range []string{"staging", "DOCKER_SSH_HOST", "DOCKER_CONTAINER_NAME", "DOCKER_SSH_PASS"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("err %v lacks %s", err, want)
		}
	}
	if _, err := NewTarget("staging", d, "key"); err == nil || strings.Contains(err.Error(), "DOCKER_SSH_PASS") {
		t.Errorf("auth key needs no password: %v", err)
	}
	if strings.Contains(fmtTarget(tg), "pw") {
		t.Error("the password is printed")
	}
}

func fmtTarget(t Target) string { return strings.Join([]string{t.String()}, "") }

func rel(version, branch string) pipeline.ReleaseOutputs {
	return pipeline.ReleaseOutputs{Status: "passed", Published: true, Version: version, Branch: branch, GitTag: "v" + version}
}

func TestSelect_Target(t *testing.T) {
	for _, tt := range []struct {
		imageVersion, version string
		want                  bool
	}{
		{"1.4.0", "1.4.0", true},
		{"v1.4.0", "1.4.0", true},
		{"1.4.0-rc.1", "1.4.0-rc.1", true},
		{"1.3.0", "1.4.0", false},
		{"latest", "1.4.0", true},
		{"latest", "1.4.0-alpha.3", false},
		{"alpha", "1.4.0-alpha.3", true},
		{"alpha", "1.4.0", false},
		{"beta", "1.4.0-rc.2", true},
		{"beta", "1.4.0-beta.1", true},
		{"beta", "1.4.0-alpha.1", false},
		{"next", "2.0.0-next.1", true},
		{"next", "1.4.0", false},
	} {
		tg := Target{Environment: "e", Enabled: true, ImageVersion: tt.imageVersion}
		d, err := Select(tg, pipelineconfig.Environment{Name: "e"}, "target", rel(tt.version, "x"))
		if err != nil || d.Deploy != tt.want || (!d.Deploy && !strings.Contains(d.Reason, tt.imageVersion)) {
			t.Errorf("%s vs %s: %+v %v, want deploy %v", tt.imageVersion, tt.version, d, err, tt.want)
		}
	}
	if _, err := Select(Target{Environment: "staging", Enabled: true}, pipelineconfig.Environment{Name: "staging"}, "target", rel("1.4.0", "main")); err == nil || !strings.Contains(err.Error(), "staging: DOCKER_IMAGE_VERSION is not set") {
		t.Errorf("missing DOCKER_IMAGE_VERSION: err = %v", err)
	}
	if d, err := Select(Target{Environment: "e", Enabled: false, ImageVersion: "1.4.0"}, pipelineconfig.Environment{Name: "e"}, "target", rel("1.4.0", "main")); err != nil || d.Deploy || !strings.Contains(d.Reason, "DOCKER_DEPLOY_ENABLED") {
		t.Errorf("disabled: %+v %v", d, err)
	}
}

func TestSelect_Branches(t *testing.T) {
	env := pipelineconfig.Environment{Name: "staging", Branches: []string{"release/*", "hotfix/*"}}
	tg := Target{Environment: "staging", Enabled: true}
	if d, err := Select(tg, env, "branches", rel("1.4.0-rc.1", "release/1.4")); err != nil || !d.Deploy {
		t.Errorf("release/1.4: %+v %v", d, err)
	}
	if d, _ := Select(tg, env, "branches", rel("1.4.0", "main")); d.Deploy || !strings.Contains(d.Reason, "main") {
		t.Errorf("main: %+v", d)
	}
	tg.Enabled = false
	if d, _ := Select(tg, env, "branches", rel("1.4.0-rc.1", "release/1.4")); d.Deploy {
		t.Errorf("disabled deploys: %+v", d)
	}
}
