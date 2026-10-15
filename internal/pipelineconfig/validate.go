package pipelineconfig

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig/yamlsection"
)

// DeploySources are the places the deploy data can come from.
var DeploySources = []string{"env", "infisical"}

// SSHAuths are the SSH authentication methods.
var SSHAuths = []string{"password", "key"}

// maxReleaseBranches is semantic-release's limit on release branches.
const maxReleaseBranches = 3

var (
	// envNameRe is a variable name as dotnet and build arguments accept it.
	envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	// secretEnvNameRe is the conventional upper-case name a *_env field
	// must hold; anything else is likely the secret value itself.
	secretEnvNameRe = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	prereleaseRe    = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z-]*$`)
	channelRe       = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	hostRe          = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*(:[0-9]+)?$`)
	imagePathRe     = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*(?:/[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*)*$`)
	platformRe      = regexp.MustCompile(`^[a-z0-9]+/[a-z0-9_]+(/[a-z0-9]+)?(,[a-z0-9]+/[a-z0-9_]+(/[a-z0-9]+)?)*$`)
	environmentRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	placeholderRe   = regexp.MustCompile(`\$\{([^}]*)\}`)
)

type validator struct{ problems []string }

func (v *validator) addf(format string, args ...any) {
	v.problems = append(v.problems, fmt.Sprintf(format, args...))
}

func (v *validator) checkPath(at, raw string) {
	if problem := yamlsection.PathProblem(at, raw); problem != "" {
		v.problems = append(v.problems, problem)
	}
}

// validate returns one path-qualified message per problem.
func (p *Pipeline) validate() []string {
	var v validator
	v.checkDotnet(p.Dotnet)
	v.checkBranches(p.Branches)
	v.checkRegistry(p.Registry)
	v.checkImages(p.Images, p.Registry.Host)
	v.checkSync(p.Sync, p.Branches)
	if p.Deploy != nil {
		v.checkDeploy(*p.Deploy)
	}
	return v.problems
}

func (v *validator) checkDotnet(d Dotnet) {
	const at = "pipeline.dotnet"
	switch {
	case strings.TrimSpace(d.Solution) == "":
		v.addf("%s.solution: required", at)
	case strings.TrimSpace(d.Solution) != d.Solution:
		v.addf("%s.solution: %q has leading or trailing spaces", at, d.Solution)
	default:
		v.checkPath(at+".solution", d.Solution)
	}
	if d.Runsettings != "" {
		v.checkPath(at+".runsettings", d.Runsettings)
	}
	for _, key := range sortedKeys(d.Env) {
		if !envNameRe.MatchString(key) {
			v.addf("%s.env.%s: not a valid environment variable name", at, key)
		}
	}
}

func (v *validator) checkBranches(branches []Branch) {
	seen := map[string]bool{}
	releases := 0
	for i, b := range branches {
		at := fmt.Sprintf("pipeline.branches[%d]", i)
		switch {
		case b.Name == "":
			v.addf("%s.name: required", at)
		case unsupportedGlob(b.Name):
			v.addf("%s.name: %q uses glob syntax nerv does not support (only *, ? and [...] are)", at, b.Name)
		case !validBranchPattern(b.Name):
			v.addf("%s.name: %q is not a valid branch name or pattern", at, b.Name)
		case seen[b.Name]:
			v.addf("%s.name: duplicate branch %q", at, b.Name)
		}
		seen[b.Name] = true
		switch {
		case b.Prerelease == "true" || b.Prerelease == "false":
			// semantic-release's "prerelease: true" uses the branch name as
			// the id, which breaks on names with a / (dim's ci-cd.yml notes
			// it for hotfix/*); a fixed id is required.
			v.addf("%s.prerelease: %s is not supported; give a fixed id such as beta", at, b.Prerelease)
		case b.Prerelease != "" && !prereleaseRe.MatchString(b.Prerelease):
			v.addf("%s.prerelease: %q is not a valid prerelease id (a letter, then letters, digits or -)", at, b.Prerelease)
		}
		switch {
		case b.Channel == "latest":
			v.addf("%s.channel: %q is reserved for the main branch image tag", at, b.Channel)
		case b.Channel != "" && !channelRe.MatchString(b.Channel):
			v.addf("%s.channel: %q is not a valid channel (it is also an image tag: letters, digits, _, . and -)", at, b.Channel)
		}
		if b.Name != "" && !b.IsPrerelease() {
			// The first release branch is the main branch (MainBranch): the
			// latest tag and the sync step use it by name, so it cannot be a
			// pattern.
			if releases == 0 && strings.ContainsAny(b.Name, "*?[") {
				v.addf("%s.name: %q is the main branch (the first release branch) and must be a branch name, not a pattern", at, b.Name)
			}
			releases++
		}
	}
	switch {
	case releases == 0:
		v.addf("pipeline.branches: at least one release branch (without prerelease) is required")
	case releases > maxReleaseBranches:
		v.addf("pipeline.branches: at most %d release branches (without prerelease) are allowed, got %d", maxReleaseBranches, releases)
	}
}

// validBranchPattern accepts a git branch name, possibly with glob
// characters (* ? [...]), as semantic-release's branches accept it.
func validBranchPattern(name string) bool {
	if strings.ContainsAny(name, " \t~^:\\") || strings.Contains(name, "..") || strings.Contains(name, "@{") ||
		strings.HasPrefix(name, "-") || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") ||
		strings.HasSuffix(name, ".lock") {
		return false
	}
	_, err := path.Match(name, "")
	return err == nil
}

// unsupportedGlob reports micromatch syntax (semantic-release's branch
// matcher) beyond what NERV matches with path.Match: globstars, braces and
// extglobs. Only *, ? and [...] mean the same in both.
func unsupportedGlob(name string) bool {
	if strings.Contains(name, "**") || strings.ContainsAny(name, "{}") {
		return true
	}
	for _, ext := range []string{"@(", "!(", "+(", "*(", "?("} {
		if strings.Contains(name, ext) {
			return true
		}
	}
	return false
}

func isPattern(name string) bool { return strings.ContainsAny(name, "*?[") }

func (v *validator) checkRegistry(r Registry) {
	const at = "pipeline.registry"
	if !hostRe.MatchString(r.Host) {
		v.addf("%s.host: %q is not a registry host such as ghcr.io", at, r.Host)
	}
	v.checkSecretEnv(at+".username_env", r.UsernameEnv)
	v.checkSecretEnv(at+".token_env", r.TokenEnv)
}

// checkSecretEnv checks a field that names the environment variable holding
// a credential. The value is never quoted: when it is not a name it may be
// the credential itself.
func (v *validator) checkSecretEnv(at, name string) {
	if !secretEnvNameRe.MatchString(name) {
		v.addf("%s: must be the name of an environment variable (A-Z, 0-9, _), not its value", at)
	}
}

func (v *validator) checkImages(images []Image, host string) {
	if len(images) == 0 {
		v.addf("pipeline.images: at least one image is required")
		return
	}
	seen := map[string]bool{}
	for i, img := range images {
		at := fmt.Sprintf("pipeline.images[%d]", i)
		if img.Name == "" {
			v.addf("%s.name: required", at)
		} else {
			rest, onHost := strings.CutPrefix(img.Name, host+"/")
			switch {
			case !onHost:
				v.addf("%s.name: %q is not on the registry host %s (pipeline.registry.host)", at, img.Name, host)
			case !imagePathRe.MatchString(rest):
				v.addf("%s.name: %q is not a valid image repository (lowercase path components, no tag or digest)", at, img.Name)
			case seen[img.Name]:
				v.addf("%s.name: duplicate image %q", at, img.Name)
			}
			seen[img.Name] = true
		}
		v.checkPath(at+".dockerfile", img.Dockerfile)
		v.checkPath(at+".context", img.Context)
		if !platformRe.MatchString(img.Platform) {
			v.addf("%s.platform: %q is not a platform such as linux/amd64", at, img.Platform)
		}
		for j, tag := range img.Tags {
			if !slices.Contains(shipconfig.DockerTags, tag) {
				v.addf("%s.tags[%d]: unknown tag %q (want %s)", at, j, tag, list(shipconfig.DockerTags))
			}
		}
		for _, key := range sortedKeys(img.BuildArgs) {
			if !envNameRe.MatchString(key) {
				v.addf("%s.build_args.%s: not a valid build argument name", at, key)
			}
			for _, m := range placeholderRe.FindAllStringSubmatch(img.BuildArgs[key], -1) {
				if !slices.Contains(shipconfig.BuildArgPlaceholders, m[1]) {
					v.addf("%s.build_args.%s: unknown placeholder ${%s} (want %s)", at, key, m[1], list(placeholders(shipconfig.BuildArgPlaceholders)))
				}
			}
		}
		for _, key := range sortedKeys(img.ComputedArgs) {
			cat := at + ".computed_args." + key
			arg := img.ComputedArgs[key]
			switch _, dup := img.BuildArgs[key]; {
			case !envNameRe.MatchString(key):
				v.addf("%s: not a valid build argument name", cat)
			case dup:
				v.addf("%s: also given in build_args", cat)
			}
			if strings.TrimSpace(arg.Run) == "" {
				v.addf("%s.run: required", cat)
			}
			if !slices.Contains(shipconfig.Shells, arg.Shell) {
				v.addf("%s.shell: unknown shell %q (want %s)", cat, arg.Shell, list(shipconfig.Shells))
			}
		}
	}
}

func (v *validator) checkSync(s Sync, branches []Branch) {
	const at = "pipeline.sync"
	if _, err := path.Match(s.Release, ""); err != nil {
		v.addf("%s.release: %q is not a valid branch pattern", at, s.Release)
	}
	if !s.IsEnabled() {
		return
	}
	if isPattern(s.Develop) {
		v.addf("%s.develop: %q must be a single branch, not a pattern", at, s.Develop)
		return
	}
	if !slices.ContainsFunc(branches, func(b Branch) bool { return b.Name == s.Develop }) {
		v.addf("%s.develop: %q is not one of pipeline.branches", at, s.Develop)
	}
}

func (v *validator) checkDeploy(d Deploy) {
	const at = "pipeline.deploy"
	if !slices.Contains(DeploySources, d.Source) {
		v.addf("%s.source: unknown source %q (want %s)", at, d.Source, list(DeploySources))
	}
	switch d.Source {
	case "infisical":
		v.checkInfisical(d.Infisical)
	case "env":
		if d.Infisical != nil {
			v.addf("%s.infisical: settings given but the source is env", at)
		}
	}
	v.checkEnvironments(d.Environments, d.Source == "env")

	ssh := d.SSH
	if !slices.Contains(SSHAuths, ssh.Auth) {
		v.addf("%s.ssh.auth: unknown auth %q (want %s)", at, ssh.Auth, list(SSHAuths))
	}
	switch {
	case ssh.Auth == "key" && strings.TrimSpace(ssh.KeyFile) == "":
		v.addf("%s.ssh.key_file: required by auth key", at)
	case ssh.Auth == "password" && ssh.KeyFile != "":
		v.addf("%s.ssh.key_file: only used by auth key", at)
	}
	if ssh.Port < 1 || ssh.Port > 65535 {
		v.addf("%s.ssh.port: %d must be between 1 and 65535", at, ssh.Port)
	}
	v.checkDuration(at+".ssh.connect_timeout", ssh.ConnectTimeout)
	v.checkDuration(at+".verify.timeout", d.Verify.Timeout)
}

func (v *validator) checkInfisical(inf *Infisical) {
	const at = "pipeline.deploy.infisical"
	if inf == nil {
		inf = &Infisical{}
	}
	if strings.TrimSpace(inf.ProjectID) == "" {
		v.addf("%s.project_id: required by source infisical", at)
	}
	switch p := inf.AppPath; {
	case p == "":
		v.addf("%s.app_path: required by source infisical", at)
	case !strings.HasPrefix(p, "/") || p == "/" || strings.HasSuffix(p, "/") || strings.ContainsAny(p, " \t\\"):
		v.addf("%s.app_path: %q must be an Infisical folder path such as /my-app", at, p)
	}
	if inf.Domain != "" {
		u, err := url.Parse(inf.Domain)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			v.addf("%s.domain: %q must be an http(s) URL", at, inf.Domain)
		}
	}
}

// checkEnvironments checks the deploy environments; with source env their
// variable prefixes must be distinct too.
func (v *validator) checkEnvironments(envs []string, fromEnv bool) {
	const at = "pipeline.deploy.environments"
	if len(envs) == 0 {
		v.addf("%s: at least one environment is required", at)
		return
	}
	seen := map[string]bool{}
	prefixes := map[string]string{}
	for i, env := range envs {
		eat := fmt.Sprintf("%s[%d]", at, i)
		switch {
		case env == "":
			v.addf("%s: required", eat)
			continue
		case !environmentRe.MatchString(env):
			v.addf("%s: %q must match [A-Za-z0-9][A-Za-z0-9_-]*", eat, env)
			continue
		case seen[env]:
			v.addf("%s: duplicate environment %q", eat, env)
			continue
		}
		seen[env] = true
		prefix := envPrefix(env)
		if other, clash := prefixes[prefix]; clash && fromEnv {
			v.addf("%s: %q and %q share the variable prefix %s", eat, env, other, prefix)
			continue
		}
		prefixes[prefix] = env
	}
}

func (v *validator) checkDuration(at, raw string) {
	switch d, err := time.ParseDuration(raw); {
	case err != nil:
		v.addf("%s: %q is not a duration such as 10s or 2m", at, raw)
	case d <= 0:
		v.addf("%s: %q must be positive", at, raw)
	}
}

// list renders "a, b or c".
func list(items []string) string {
	if len(items) == 1 {
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}

// placeholders renders names as ${name}.
func placeholders(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "${" + n + "}"
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
