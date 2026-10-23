package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// requestFor is the deploy-provider request nerv deploy sends.
func requestFor(op string, config map[string]string) string {
	cfg := map[string]string{
		"environment": "staging", "source": "env", "ssh_auth": "password", "ssh_known_hosts": "~/.ssh/known_hosts",
		"ssh_port": "22", "ssh_connect_timeout": "10s", "update_timeout": "15m", "verify_healthy": "true",
		"version_label": "org.opencontainers.image.version", "verify_timeout": "60s",
	}
	for k, v := range config {
		cfg[k] = v
	}
	data, _ := json.Marshal(map[string]any{
		"contract_version": 1, "operation": op, "target": "staging",
		"artifact": map[string]any{"version": "1.4.0", "branch": "main", "tag": "v1.4.0"}, "config": cfg,
	})
	return string(data)
}

func stagingEnv(extra map[string]string) func(string) string {
	vars := map[string]string{
		"NERV_DEPLOY_STAGING_DOCKER_SSH_HOST": "10.0.0.1", "NERV_DEPLOY_STAGING_DOCKER_SSH_USER": "deploy",
		"NERV_DEPLOY_STAGING_DOCKER_SSH_PASS": "s3cr3t", "NERV_DEPLOY_STAGING_DOCKER_COMPOSE_PATH": "/srv/apps",
		"NERV_DEPLOY_STAGING_DOCKER_COMPOSE_SCRIPT": "./manage.sh", "NERV_DEPLOY_STAGING_DOCKER_CONTAINER_NAME": "api",
		"NERV_DEPLOY_STAGING_DOCKER_CONTAINER_VARIANT": "staging",
	}
	for k, v := range extra {
		vars[k] = v
	}
	return func(k string) string { return vars[k] }
}

func runCompose(t *testing.T, op, req string, f *fakeProc, getenv func(string) string) (map[string]string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := RunComposeOp(context.Background(), op, strings.NewReader(req), &out, ComposeDeps{
		Proc: f, Getenv: getenv, Home: "/home/u", Output: &errOut,
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	var resp map[string]string
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("response %q: %v (stderr %s)", out.String(), err, errOut.String())
	}
	if strings.Contains(out.String()+errOut.String(), "s3cr3t") {
		t.Errorf("the password was printed")
	}
	return resp, code
}

// deploy runs the compose script's pull and up only; verify checks the
// health and the version label only.
func TestRunComposeOp_DeployUpdatesAndVerifyChecks(t *testing.T) {
	h := &host{statuses: []string{"Up 40 seconds (healthy)"}, label: "1.4.0"}
	f := &fakeProc{answer: h.answer}
	env := stagingEnv(map[string]string{"NERV_DEPLOY_STAGING_DOCKER_IMAGE_VERSION": "1.4.0"})
	resp, code := runCompose(t, "deploy", requestFor("deploy", nil), f, env)
	if code != 0 || resp["status"] != "ok" || len(f.cmds) != 1 || len(h.updates) != 1 {
		t.Fatalf("deploy: %v code %d, %d commands", resp, code, len(f.cmds))
	}
	if kh := f.cmds[0].Args; !strings.Contains(strings.Join(kh, " "), `UserKnownHostsFile="/home/u/.ssh/known_hosts"`) {
		t.Errorf("known_hosts not expanded: %q", kh)
	}
	f.cmds = nil
	resp, code = runCompose(t, "verify", requestFor("verify", nil), f, env)
	if code != 0 || resp["status"] != "ok" || len(h.updates) != 1 || !strings.Contains(resp["message"], "1.4.0") {
		t.Fatalf("verify: %v code %d, updates %d", resp, code, len(h.updates))
	}

	h.label = "1.3.9"
	resp, code = runCompose(t, "verify", requestFor("verify", nil), f, env)
	if code != 1 || resp["status"] != "failed" || !strings.Contains(resp["message"], "runs 1.3.9, not 1.4.0") {
		t.Errorf("wrong version: %v code %d", resp, code)
	}
}

// A disabled environment, or one whose DOCKER_IMAGE_VERSION points at
// other releases, is skipped; bad config or missing data fails.
func TestRunComposeOp_SkipsAndRefuses(t *testing.T) {
	f := &fakeProc{}
	resp, code := runCompose(t, "deploy", requestFor("deploy", nil), f, stagingEnv(map[string]string{"NERV_DEPLOY_STAGING_DOCKER_DEPLOY_ENABLED": "false"}))
	if code != 0 || resp["status"] != "skipped" || len(f.cmds) != 0 {
		t.Errorf("disabled: %v code %d", resp, code)
	}
	resp, _ = runCompose(t, "deploy", requestFor("deploy", nil), f, stagingEnv(map[string]string{"NERV_DEPLOY_STAGING_DOCKER_IMAGE_VERSION": "alpha"}))
	if resp["status"] != "skipped" || !strings.Contains(resp["message"], "does not point at 1.4.0") {
		t.Errorf("other selection: %v", resp)
	}
	resp, code = runCompose(t, "deploy", requestFor("deploy", map[string]string{"ssh_auth": "kerberos"}), f, stagingEnv(nil))
	if code != 1 || !strings.Contains(resp["message"], "ssh_auth") {
		t.Errorf("bad auth: %v", resp)
	}
	resp, code = runCompose(t, "deploy", requestFor("deploy", nil), f, func(string) string { return "" })
	if code != 1 || !strings.Contains(resp["message"], "missing DOCKER_SSH_HOST") {
		t.Errorf("no data: %v", resp)
	}
	resp, code = runCompose(t, "verify", requestFor("deploy", nil), f, stagingEnv(nil))
	if code != 1 || !strings.Contains(resp["message"], "operation") {
		t.Errorf("mismatched operation: %v", resp)
	}
	if len(f.cmds) != 0 {
		t.Errorf("a refused operation connected: %d commands", len(f.cmds))
	}
}
