package migrate

import (
	"bytes"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/deployprovider"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig/yamlsection"
	"gopkg.in/yaml.v3"
)

// The deploy providers nerv migrate writes.
const (
	sshComposeProvider = "ssh-compose"
	githubHost         = "github"
)

// oldEnvironments are 4.2.0's default deploy environments.
var oldEnvironments = []string{"production-green", "production-blue", "staging", "development"}

// targetNameRe is deployconfig's target name rule.
var targetNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// deployTarget is one deploy-ssh-compose target.
type deployTarget struct {
	name   string
	config map[string]string
}

// deployTranslation is the deploy: section's content.
type deployTranslation struct {
	host    string
	targets []deployTarget
}

// oldDeploySource maps each 4.2.0 pipeline.deploy key (a dotted path below
// pipeline.deploy) to its deploy-ssh-compose config key.
var oldDeploySource = []struct{ from, to string }{
	{"source", "source"},
	{"infisical.project_id", "infisical_project_id"},
	{"infisical.app_path", "infisical_app_path"},
	{"infisical.domain", "infisical_domain"},
	{"update_timeout", "update_timeout"},
	{"ssh.auth", "ssh_auth"},
	{"ssh.key_file", "ssh_key_file"},
	{"ssh.known_hosts", "ssh_known_hosts"},
	{"ssh.port", "ssh_port"},
	{"ssh.connect_timeout", "ssh_connect_timeout"},
	{"verify.healthy", "verify_healthy"},
	{"verify.version_label", "version_label"},
	{"verify.timeout", "verify_timeout"},
}

// sshComposeManifest is the deploy-ssh-compose provider's manifest.
func sshComposeManifest(fsys fs.FS) (*deployprovider.Manifest, error) {
	data, err := fs.ReadFile(fsys, "deploy-"+sshComposeProvider+"/"+deployprovider.ManifestFile)
	if err != nil {
		return nil, err
	}
	return deployprovider.Parse(data)
}

// translatePipeline turns the 4.2.0 pipeline: section into deploy targets:
// one deploy-ssh-compose target per environment of pipeline.deploy, with
// the settings written there. The user-scope settings go to the user
// configuration with user, otherwise into each target, reported. What has
// no equivalent is reported.
func translatePipeline(p *Plan, data []byte, fsys fs.FS, user *userState) (*deployTranslation, error) {
	node, err := yamlsection.Locate(data, "pipeline")
	if err != nil {
		return nil, refuse("pipeline: cannot be read: %v", err)
	}
	out := &deployTranslation{}
	if node.Kind != yaml.MappingNode {
		p.warn("pipeline: not a mapping, dropped")
		return out, nil
	}
	if value(node, "dotnet") != nil {
		p.warn("pipeline.dotnet (the validate and test steps): already covered by nerv ship, the stages of each ship: project; dropped")
	}
	if b := value(node, "branches"); b != nil {
		p.warn("pipeline.branches: the release branches are the ship: section's branches (imported from the semantic-release configuration); check them with nerv version next")
	}
	if r := value(node, "registry"); r != nil {
		p.warn("pipeline.registry: deploy-ssh-compose pushes with your docker login: run docker login %s before nerv deploy (username_env and token_env have no equivalent)", orDefault(scalarValue(value(r, "host")), "ghcr.io"))
	}
	if imgs := value(node, "images"); imgs != nil && imgs.Kind == yaml.SequenceNode {
		var names []string
		for _, img := range imgs.Content {
			names = append(names, scalarValue(value(img, "name")))
		}
		p.warn("pipeline.images (%s): nerv ship builds the images now: give the ship: project build.mode: compose with build.images (or a package stage that docker saves them into $NERV_RELEASE_OUTPUTS_DIR), and deploy-ssh-compose pushes them", strings.Join(names, ", "))
	}
	if sync := value(node, "sync"); scalarValue(value(sync, "enabled")) != "false" {
		p.warn("pipeline.sync (on unless sync.enabled: false): not ported: nerv deploy does no back-merge after a release; merge the released branch back into develop by hand")
	}
	if rel := value(node, "release"); rel != nil && value(rel, "version_files") != nil {
		p.warn("pipeline.release.version_files: write them as build.version_files of the ship: project")
	}
	dep := value(node, "deploy")
	if dep == nil {
		return out, nil
	}
	m, err := sshComposeManifest(fsys)
	if err != nil {
		return nil, err
	}
	if sel := scalarValue(value(dep, "selection")); sel == "branches" {
		p.warn("pipeline.deploy.selection: branches has no equivalent: a deploy-ssh-compose target deploys its environment when the environment's DOCKER_IMAGE_VERSION points at the release; one target is written per environment: choose them with nerv deploy --target")
	}
	shared := map[string]string{}
	for _, k := range oldDeploySource {
		n := dep
		for _, seg := range strings.Split(k.from, ".") {
			n = value(n, seg)
		}
		if n == nil || n.Kind != yaml.ScalarNode || n.Value == "" {
			continue
		}
		shared[k.to] = n.Value
	}
	for _, key := range slices.Sorted(maps.Keys(shared)) {
		c, ok := m.Key(key)
		if !ok || c.Scope != "user" {
			continue
		}
		path := "deploy.providers." + sshComposeProvider + "." + key
		switch {
		case user == nil:
			p.warn("deploy.targets[*].config.%s: a personal setting (scope user) written into each target; run nerv migrate --user here to keep it in the user config instead", key)
		case func() bool { _, set := user.doc.Scalar(path); return set }():
			existing, _ := user.doc.Scalar(path)
			p.warn("deploy.targets[*].config.%s: kept in each target, the user config already sets %s to %s", key, path, existing)
		default:
			user.doc.SetScalar(path, shared[key])
			p.note("pipeline.deploy: %s moved to the user config as %s", key, path)
			delete(shared, key)
		}
	}
	envs := oldEnvironments
	if e := value(dep, "environments"); e != nil && e.Kind == yaml.SequenceNode {
		envs = nil
		for _, item := range e.Content {
			if item.Kind == yaml.ScalarNode {
				envs = append(envs, item.Value)
			} else {
				envs = append(envs, scalarValue(value(item, "name")))
			}
		}
	}
	for _, env := range envs {
		name := env
		if !targetNameRe.MatchString(name) {
			name = strings.Trim(regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(strings.ToLower(env), "-"), "-.")
			p.warn("pipeline.deploy.environments: %q is not a target name; the target is %q", env, name)
		}
		cfg := maps.Clone(shared)
		cfg["environment"] = env
		out.targets = append(out.targets, deployTarget{name: name, config: cfg})
	}
	p.note("pipeline.deploy: one deploy-ssh-compose target per environment (%s)", strings.Join(envs, ", "))
	return out, nil
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

// isGitHub reports whether an origin URL is a GitHub repository.
func isGitHub(origin string) bool {
	o := strings.ToLower(origin)
	return strings.Contains(o, "github.com/") || strings.Contains(o, "github.com:")
}

// deployBlock is the "deploy:" block text of d.
func deployBlock(d *deployTranslation) (string, error) {
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if d.host != "" {
		addKV(m, "host", scalar(d.host))
	}
	if len(d.targets) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, t := range d.targets {
			tn := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			addKV(tn, "name", scalar(t.name))
			addKV(tn, "provider", scalar(sshComposeProvider))
			cfg := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			for _, k := range slices.Sorted(maps.Keys(t.config)) {
				addKV(cfg, k, scalar(t.config[k]))
			}
			addKV(tn, "config", cfg)
			seq.Content = append(seq.Content, tn)
		}
		addKV(m, "targets", seq)
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{scalar("deploy"), m}}
	if err := enc.Encode(root); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return strings.TrimRight(b.String(), "\n"), nil
}
