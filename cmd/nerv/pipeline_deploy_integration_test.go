//go:build semrel_integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
)

// localSSHD starts the system's sshd as the current user on a free
// 127.0.0.1 port (loopback only, no root: it can only log in as this user,
// with a key), whose remote commands see bin first in PATH. It returns the
// port, the client key and the host key line for known_hosts; it skips
// without sshd, ssh or ssh-keygen.
func localSSHD(t *testing.T, bin string) (port int, clientKey, hostLine string) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test")
	}
	if runtime.GOOS != "linux" {
		t.Skip("the local sshd runs on Linux only")
	}
	sshd := "/usr/sbin/sshd"
	for _, tool := range []string{sshd, "ssh", "ssh-keygen"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	dir := t.TempDir()
	keygen := func(path string) {
		if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", path).CombinedOutput(); err != nil {
			t.Fatalf("ssh-keygen: %v %s", err, out)
		}
	}
	hostKey, clientKey := filepath.Join(dir, "host_key"), filepath.Join(dir, "client_key")
	keygen(hostKey)
	keygen(clientKey)
	pub, err := os.ReadFile(clientKey + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "authorized_keys"), pub, 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port = l.Addr().(*net.TCPAddr).Port
	l.Close()
	conf := filepath.Join(dir, "sshd_config")
	if err := os.WriteFile(conf, []byte(strings.Join([]string{
		"ListenAddress 127.0.0.1", "Port " + strconv.Itoa(port), "HostKey " + hostKey,
		"AuthorizedKeysFile " + filepath.Join(dir, "authorized_keys"), "PidFile " + filepath.Join(dir, "sshd.pid"),
		"UsePAM no", "PasswordAuthentication no", "KbdInteractiveAuthentication no", "StrictModes no",
		"SetEnv PATH=" + bin + ":/usr/bin:/bin", "LogLevel ERROR",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, sshd, "-D", "-e", "-f", conf)
	var log bytes.Buffer
	cmd.Stdout, cmd.Stderr = &log, &log
	if err := cmd.Start(); err != nil {
		cancel()
		t.Skipf("sshd: %v", err)
	}
	t.Cleanup(func() { cancel(); cmd.Wait() })
	for i := 0; i < 50; i++ {
		if c, err := net.DialTimeout("tcp", l.Addr().String(), 200*time.Millisecond); err == nil {
			c.Close()
			hostPub, err := os.ReadFile(hostKey + ".pub")
			if err != nil {
				t.Fatal(err)
			}
			fields := strings.Fields(string(hostPub))
			return port, clientKey, "[127.0.0.1]:" + strconv.Itoa(port) + " " + fields[0] + " " + fields[1]
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Skipf("sshd did not start: %s", log.String())
	return
}

// TestIntegration_PipelineDeployOverARealSSHD: real ssh and sshd on
// loopback (key auth, strict host keys). The host's compose script and
// docker are fakes in a temporary bin: the script records its calls, docker
// answers healthy with the released version. An unpinned host key fails
// before anything runs.
func TestIntegration_PipelineDeployOverARealSSHD(t *testing.T) {
	shipEnv(t)
	bin := t.TempDir()
	apps := t.TempDir()
	calls := filepath.Join(apps, "calls.log")
	write := func(path, content string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(apps, "manage.sh"), "#!/bin/sh\necho \"manage.sh $*\" >> "+calls+"\necho \"$1 done\"\n", 0o755)
	write(filepath.Join(bin, "docker"), `#!/bin/sh
case "$1" in
  ps) echo "Up 12 seconds (healthy)" ;;
  inspect) echo "1.4.0-rc.1" ;;
  *) echo "unexpected docker $*" >&2; exit 1 ;;
esac
`, 0o755)
	port, key, hostLine := localSSHD(t, bin)

	work := gitDir{t, filepath.Join(t.TempDir(), "api")}
	if out, err := exec.Command("git", "init", "-q", "-b", "main", work.dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	work.write(".nerv/nerv.yaml", `pipeline:
  dotnet: { solution: App.slnx }
  images: [{ name: ghcr.io/acme/api }]
  deploy:
    environments: [production, staging]
    ssh: { auth: key, key_file: "`+key+`", known_hosts: "`+knownHosts+`", port: `+strconv.Itoa(port)+`, connect_timeout: 5s }
    verify: { timeout: 30s }
`)
	user := os.Getenv("USER")
	if user == "" {
		out, _ := exec.Command("id", "-un").Output()
		user = strings.TrimSpace(string(out))
	}
	for env, version := range map[string]string{"PRODUCTION": "latest", "STAGING": "beta"} {
		p := "NERV_DEPLOY_" + env + "_"
		for k, v := range map[string]string{"DOCKER_SSH_HOST": "127.0.0.1", "DOCKER_SSH_USER": user, "DOCKER_COMPOSE_PATH": apps,
			"DOCKER_COMPOSE_SCRIPT": "./manage.sh", "DOCKER_CONTAINER_NAME": "api", "DOCKER_CONTAINER_VARIANT": strings.ToLower(env), "DOCKER_IMAGE_VERSION": version} {
			t.Setenv(p+k, v)
		}
	}
	rel := pipeline.ReleaseOutputs{Step: "release", Status: "passed", Published: true, Branch: "release/1.4", Version: "1.4.0-rc.1", GitTag: "v1.4.0-rc.1", GitHead: "c0ffee", BaseCommit: "base0"}
	if _, err := pipeline.WriteReleaseOutputs(work.dir, rel); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.WriteDockerResult(work.dir, pipeline.DockerResult{Step: "docker", Status: "passed", Version: rel.Version, GitTag: rel.GitTag, Revision: rel.GitHead, Images: []pipeline.ImageResult{}}); err != nil {
		t.Fatal(err)
	}
	deployRun := func() (int, pipeline.DeployResult, string) {
		t.Helper()
		opts := defaultOptions()
		opts.Stdin = strings.NewReader("")
		var stdout, stderr bytes.Buffer
		start := time.Now()
		code := run([]string{"pipeline", "deploy", "--cwd", work.dir, "--yes", "--json"}, &stdout, &stderr, opts)
		t.Logf("deploy: exit %d in %s", code, time.Since(start).Round(time.Millisecond))
		var res pipeline.DeployResult
		if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
			t.Fatalf("%v\nstdout %s\nstderr %s", err, stdout.String(), stderr.String())
		}
		return code, res, stderr.String()
	}

	// The host key is not pinned yet: strict checking refuses the host.
	write(knownHosts, "", 0o600)
	code, res, stderr := deployRun()
	if code != 2 || res.Environments[1].Status != "failed" || !strings.Contains(res.Environments[1].Error, "ssh could not connect") {
		t.Fatalf("unpinned host: exit %d %+v\n%s", code, res.Environments, stderr)
	}
	if _, err := os.Stat(calls); err == nil {
		t.Fatal("the compose script ran on an unpinned host")
	}

	write(knownHosts, hostLine+"\n", 0o600)
	code, res, stderr = deployRun()
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if got := statuses(res.Environments); strings.Join(got, ",") != "production:skipped,staging:deployed" || res.Environments[1].Running != "1.4.0-rc.1" || res.Environments[1].Health != "Up 12 seconds (healthy)" {
		t.Errorf("result %+v", res.Environments)
	}
	data, err := os.ReadFile(calls)
	if err != nil || string(data) != "manage.sh pull --container api-staging\nmanage.sh up --container api-staging\n" {
		t.Errorf("compose script calls %q %v", data, err)
	}
	if !strings.Contains(stderr, "[pipeline:deploy] pull done") {
		t.Errorf("the compose script's output is not streamed:\n%s", stderr)
	}
}
