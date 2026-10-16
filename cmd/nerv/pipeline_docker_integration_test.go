//go:build semrel_integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
)

// dockerCmd runs docker and returns its trimmed combined output.
func dockerCmd(t *testing.T, timeout time.Duration, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// httpClient bounds every request to the local registry.
var httpClient = &http.Client{Timeout: 5 * time.Second}

// localRegistry starts registry:2 bound to a free 127.0.0.1 port and
// returns host:port; it skips without docker or the image. It uses the
// host network with REGISTRY_HTTP_ADDR on loopback: on this host a
// published port (-p 127.0.0.1::5000) accepted connections but never
// answered.
func localRegistry(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test")
	}
	if runtime.GOOS != "linux" {
		t.Skip("the local registry uses the host network (Linux only)")
	}
	if _, err := dockerCmd(t, 20*time.Second, "info"); err != nil {
		t.Skipf("docker not reachable: %v", err)
	}
	if _, err := dockerCmd(t, 20*time.Second, "image", "inspect", "registry:2"); err != nil {
		if out, err := dockerCmd(t, 3*time.Minute, "pull", "registry:2"); err != nil {
			t.Skipf("registry:2 unavailable: %v %s", err, out)
		}
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := l.Addr().String()
	l.Close()
	id, err := dockerCmd(t, time.Minute, "run", "-d", "--rm", "--network", "host", "-e", "REGISTRY_HTTP_ADDR="+host, "registry:2")
	if err != nil {
		t.Fatalf("starting registry:2: %v %s", err, id)
	}
	t.Cleanup(func() { dockerCmd(t, time.Minute, "rm", "-f", id) })
	for i := 0; i < 50; i++ {
		if resp, err := httpClient.Get("http://" + host + "/v2/"); err == nil {
			resp.Body.Close()
			return host
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("registry %s did not start", host)
	return ""
}

func TestIntegration_PipelineDockerPushesToALocalRegistry(t *testing.T) {
	registry := localRegistry(t)
	shipEnv(t)
	origin, work := newShipRepos(t)
	work.write("Dockerfile", "FROM scratch\nARG APP_VERSION\nARG STAMP\nCOPY version.txt /version.txt\nLABEL app.version=$APP_VERSION app.stamp=$STAMP\n")
	work.write("version.txt", "1.0.0\n")
	work.commit("feat: app", "a.txt")
	work.git("tag", "v1.0.0")
	work.git("push", "-q", "origin", "main", "--tags")
	commit := origin.git("rev-parse", "v1.0.0^{commit}")
	image := registry + "/acme/app"
	work.write(".nerv/nerv.yaml", `pipeline:
  dotnet:
    solution: App.slnx
  registry:
    host: `+registry+`
  images:
    - name: `+image+`
      title: acme.app
      build_args:
        APP_VERSION: "${version}"
      computed_args:
        STAMP: { run: "echo stamp-1" }
`)
	if _, err := pipeline.WriteReleaseOutputs(work.dir, pipeline.ReleaseOutputs{Step: "release", Status: "passed", Published: true,
		Branch: "main", Version: "1.0.0", GitTag: "v1.0.0", GitHead: commit}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dockerCmd(t, time.Minute, "rmi", "-f", image+":1.0.0", image+":latest") })

	opts := defaultOptions()
	opts.Stdin = strings.NewReader("")
	var stdout, stderr bytes.Buffer
	start := time.Now()
	code := run([]string{"pipeline", "docker", "--cwd", work.dir, "--no-login", "--json"}, &stdout, &stderr, opts)
	t.Logf("docker step: exit %d in %s", code, time.Since(start).Round(time.Millisecond))
	if code != 0 {
		t.Fatalf("exit %d\nstdout %s\nstderr %s", code, stdout.String(), stderr.String())
	}
	var res pipeline.DockerResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	img := res.Images[0]
	if img.Status != "pushed" || !slices.Equal(img.Tags, []string{"1.0.0", "latest"}) || !strings.HasPrefix(img.Digest, "sha256:") || res.Revision != commit {
		t.Errorf("result %+v", res)
	}

	resp, err := httpClient.Get("http://" + registry + "/v2/acme/app/tags/list")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list struct{ Tags []string }
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	slices.Sort(list.Tags)
	if !slices.Equal(list.Tags, []string{"1.0.0", "latest"}) {
		t.Errorf("registry tags %v", list.Tags)
	}
	labels, err := dockerCmd(t, 20*time.Second, "image", "inspect", "--format", "{{json .Config.Labels}}", image+":1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	var l map[string]string
	if err := json.Unmarshal([]byte(labels), &l); err != nil {
		t.Fatal(err)
	}
	if l["org.opencontainers.image.revision"] != commit || l["org.opencontainers.image.version"] != "1.0.0" || l["org.opencontainers.image.title"] != "acme.app" || l["app.version"] != "1.0.0" || l["app.stamp"] != "stamp-1" {
		t.Errorf("labels %v", l)
	}
	if _, err := pipeline.ReadReleaseOutputs(work.dir); err != nil {
		t.Errorf("release.json: %v", err)
	}
	if _, err := dockerCmd(t, 20*time.Second, "image", "inspect", image+":latest"); err != nil {
		t.Errorf("latest not built locally: %v", err)
	}
	if data := filepath.Join(work.dir, ".nerv", "artifacts", "pipeline", "docker.json"); !strings.Contains(stderr.String(), data) {
		t.Errorf("result path not reported:\n%s", stderr.String())
	}
}
