package deployrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// dockerHub is the registry of an image reference with no registry host,
// and the names Docker Hub's credentials are stored under.
const dockerHub = "docker.io"

var dockerHubKeys = []string{"docker.io", "index.docker.io", "registry-1.docker.io"}

// publishes reports whether the core publish step has images to push.
func (d *deployer) publishes() bool {
	return d.o.Config.Publishes() && len(d.m.Images) > 0
}

// publish is the core publish step: docker load of every image tar of the
// artifact, then docker push of every image of its manifest. Re-pushing a
// tag the registry already has with the same digest uploads nothing, so a
// re-run is safe.
func (d *deployer) publish(ctx context.Context) error {
	tr := d.tr
	switch {
	case !d.o.Config.Publishes():
		tr.set("publish", StatusSkipped, "deploy.publish is false: the targets publish")
		return nil
	case len(d.m.Images) == 0:
		tr.set("publish", StatusSkipped, "the artifact has no images")
		return nil
	}
	for _, out := range d.m.Outputs {
		if out.Kind != artifact.KindImageTar {
			continue
		}
		if err := d.docker(ctx, "load", "-i", filepath.Join(d.o.ArtifactDir, filepath.FromSlash(out.Path))); err != nil {
			tr.set("publish", StatusFailed, "%v", err)
			return err
		}
	}
	for _, image := range d.m.Images {
		if err := d.docker(ctx, "push", image); err != nil {
			tr.set("publish", StatusFailed, "%v", err)
			return err
		}
	}
	tr.set("publish", StatusDone, "pushed %s", strings.Join(d.m.Images, " "))
	return nil
}

// docker runs docker with args, its output going to the progress. A
// failure carries the last line docker wrote.
func (d *deployer) docker(ctx context.Context, args ...string) error {
	res, err := d.o.Proc.Run(ctx, proc.Command{Name: "docker", Args: args, Env: d.o.ProcEnv, Stdout: d.o.Output, Stderr: d.o.Output})
	if err == nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(res.Stdout)), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return fmt.Errorf("docker %s: %v: %s", strings.Join(args, " "), err, last)
	}
	return fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
}

// checkCredentials refuses the deploy when an image's registry has no
// credentials in the docker config, before anything is written.
func (d *deployer) checkCredentials() error {
	env := d.o.ProcEnv
	if env == nil {
		env = os.Environ()
	}
	missing, err := registriesWithoutCredentials(d.m.Images, env)
	if err != nil {
		return refuse("deploy.publish: %v (nothing was written)", err)
	}
	if len(missing) == 0 {
		return nil
	}
	var steps []string
	for _, r := range missing {
		login := "docker login " + r
		if r == dockerHub {
			login = "docker login"
		}
		steps = append(steps, fmt.Sprintf("no credentials for %s in the docker config: run %s", r, login))
	}
	return refuse("deploy.publish: %s, or set deploy.publish: false (nothing was written)", strings.Join(steps, "; "))
}

// missingCredentials is registriesWithoutCredentials without its error.
func missingCredentials(images, env []string) []string {
	missing, _ := registriesWithoutCredentials(images, env)
	return missing
}

// registriesWithoutCredentials are the registries of images, in order and
// once each, that the docker config ($DOCKER_CONFIG/config.json, else
// ~/.docker/config.json) has neither an auths entry nor a credHelpers
// entry for. A missing config file has none.
func registriesWithoutCredentials(images, env []string) ([]string, error) {
	known, err := dockerCredentialRegistries(env)
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, image := range images {
		r := imageRegistry(image)
		have := known[r]
		if r == dockerHub {
			for _, k := range dockerHubKeys {
				have = have || known[k]
			}
		}
		if !have && !slices.Contains(missing, r) {
			missing = append(missing, r)
		}
	}
	return missing, nil
}

// dockerCredentialRegistries are the registry hosts the docker config has
// credentials for.
func dockerCredentialRegistries(env []string) (map[string]bool, error) {
	dir := lookupEnv(env, "DOCKER_CONFIG")
	if dir == "" {
		home := lookupEnv(env, "HOME")
		if home == "" {
			return map[string]bool{}, nil
		}
		dir = filepath.Join(home, ".docker")
	}
	path := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Auths       map[string]json.RawMessage `json:"auths"`
		CredHelpers map[string]string          `json:"credHelpers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	known := map[string]bool{}
	for k := range cfg.Auths {
		known[registryHost(k)] = true
	}
	for k := range cfg.CredHelpers {
		known[registryHost(k)] = true
	}
	return known, nil
}

// registryHost is the host of a docker config key: "https://ghcr.io",
// "ghcr.io/v2/" and "ghcr.io" are all "ghcr.io".
func registryHost(key string) string {
	key = strings.TrimPrefix(strings.TrimPrefix(key, "https://"), "http://")
	host, _, _ := strings.Cut(key, "/")
	return host
}

// imageRegistry is the registry host of an image reference, as docker reads
// it: the first path component when it has a dot or a port or is
// localhost, else Docker Hub.
func imageRegistry(image string) string {
	first, _, found := strings.Cut(image, "/")
	if found && (strings.ContainsAny(first, ".:") || first == "localhost") {
		return first
	}
	return dockerHub
}

// lookupEnv is the value of key in env (KEY=value entries; the last wins).
func lookupEnv(env []string, key string) string {
	value := ""
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			value = v
		}
	}
	return value
}
