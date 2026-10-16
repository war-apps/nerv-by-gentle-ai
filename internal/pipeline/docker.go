package pipeline

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shiprun"
)

// ArgRunner computes a build argument: *shiprun.Runner is one (Capture).
type ArgRunner interface {
	Capture(ctx context.Context, step shiprun.Step, vars shiprun.Vars) (string, error)
}

// Credentials log docker in to the registry; the token travels on the
// login's stdin only.
type Credentials struct {
	Host, User, Token string
}

// Docker is dim's docker job: every pipeline.images entry built from the
// released commit, tagged, labelled and pushed.
type Docker struct {
	// Dir is the release clone, checked out at the released commit.
	Dir     string
	Config  *pipelineconfig.Pipeline
	Release ReleaseOutputs
	// Revision is the built commit (org.opencontainers.image.revision).
	Revision string
	// Source is origin's public address (org.opencontainers.image.source);
	// empty leaves the label out.
	Source string
	// Proc runs docker; Args computes build arguments.
	Proc proc.Runner
	Args ArgRunner
	// Output receives the commands' output (already prefixed by the caller).
	Output io.Writer
	// Login, when set (and not a dry run), logs in before building.
	Login *Credentials
	// DryRun builds but never logs in or pushes.
	DryRun bool
	// Env is docker's environment before SafeEnv filters it; nil means
	// os.Environ().
	Env []string
}

// SafeEnv is env without the pipeline's secrets: the registry's
// username_env and token_env variables and every NERV_DEPLOY_* variable.
// Everything else docker needs (PATH, HOME, DOCKER_CONFIG, DOCKER_HOST,
// proxies) is kept. Names compare case-insensitively (Windows).
func SafeEnv(env []string, r pipelineconfig.Registry) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		upper := strings.ToUpper(name)
		if (r.UsernameEnv != "" && strings.EqualFold(name, r.UsernameEnv)) ||
			(r.TokenEnv != "" && strings.EqualFold(name, r.TokenEnv)) ||
			strings.HasPrefix(upper, pipelineconfig.DeployEnvPrefix+"_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// gitHubTokens are the GitHub tokens semantic-release and gh read; the
// dotnet steps never need them.
var gitHubTokens = []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"}

// BuildEnv is SafeEnv without the GitHub tokens too: the environment of
// the validate and test steps' dotnet commands, which build and run the
// repository's own code and tests.
func BuildEnv(env []string, r pipelineconfig.Registry) []string {
	out := SafeEnv(env, r)
	return slices.DeleteFunc(out, func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.ContainsFunc(gitHubTokens, func(t string) bool { return strings.EqualFold(t, name) })
	})
}

// ImageResult is what happened to one image.
type ImageResult struct {
	Name     string   `json:"name"`
	Title    string   `json:"title,omitempty"`
	Tags     []string `json:"tags"`
	Refs     []string `json:"refs"`
	Digest   string   `json:"digest,omitempty"`
	Status   string   `json:"status"` // built (dry run), pushed, failed
	Attempts int      `json:"attempts"`
	Required bool     `json:"required"`
	Error    string   `json:"error,omitempty"`
}

// DockerResult is the docker step's result (docker.json).
type DockerResult struct {
	Step     string        `json:"step"`
	Status   string        `json:"status"`
	DryRun   bool          `json:"dry_run"`
	Version  string        `json:"version,omitempty"`
	GitTag   string        `json:"git_tag,omitempty"`
	Revision string        `json:"revision,omitempty"`
	Images   []ImageResult `json:"images"`
	Error    string        `json:"error,omitempty"`
}

// ImageTags are the tags of an image (dim's docker/metadata-action rules),
// in this order: the version always, the channel unless it is empty or
// "default", latest only on the main branch; selected by the image's tags.
func ImageTags(selected []string, version, channel string, mainBranch bool) []string {
	var out []string
	add := func(kind, value string, on bool) {
		if on && slices.Contains(selected, kind) && !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	add("version", version, true)
	add("channel", channel, channel != "" && channel != "default")
	add("latest", "latest", mainBranch)
	return out
}

// Run builds every image in order, then pushes its tags (not in a dry
// run). A failed build is retried once, with the computed arguments
// computed again, when the image has any (dim's retry). An image with
// required false may fail without failing the step; every image is tried.
func (d Docker) Run(ctx context.Context) DockerResult {
	r := DockerResult{Step: "docker", Status: StatusPassed, DryRun: d.DryRun, Version: d.Release.Version, GitTag: d.Release.GitTag, Revision: d.Revision, Images: []ImageResult{}}
	if d.Login != nil && !d.DryRun {
		if _, err := d.docker(ctx, []byte(d.Login.Token+"\n"), "login", d.Login.Host, "-u", d.Login.User, "--password-stdin"); err != nil {
			r.Status, r.Error = StatusFailed, fmt.Sprintf("docker login %s: %v", d.Login.Host, err)
			return r
		}
	}
	main := d.Release.Branch == d.Config.MainBranch()
	var failed []string
	for _, img := range d.Config.Images {
		res := d.image(ctx, img, main)
		r.Images = append(r.Images, res)
		if res.Status == "failed" && res.Required {
			// The cause goes with the name: docker.json is not written on a
			// failure, so the step's document is the only record of it.
			failed = append(failed, img.Name+": "+res.Error)
		}
		if ctx.Err() != nil {
			r.Status, r.Error = StatusInterrupted, ctx.Err().Error()
			return r
		}
	}
	if len(failed) > 0 {
		r.Status, r.Error = StatusFailed, "required image(s) failed: "+strings.Join(failed, "; ")
	}
	return r
}

var digestRe = regexp.MustCompile(`digest: (sha256:[0-9a-f]{64})`)

func (d Docker) image(ctx context.Context, img pipelineconfig.Image, main bool) ImageResult {
	rel := d.Release
	res := ImageResult{Name: img.Name, Title: img.Title, Required: img.IsRequired(), Tags: ImageTags(img.Tags, rel.Version, rel.Channel, main), Refs: []string{}}
	for _, t := range res.Tags {
		res.Refs = append(res.Refs, img.Name+":"+t)
	}
	fail := func(err error) ImageResult {
		res.Status, res.Error = "failed", err.Error()
		return res
	}
	if len(res.Refs) == 0 {
		// Never build an untagged image or report it pushed.
		return fail(fmt.Errorf("no tag selected for %s on %s (tags %v; channel %q)", img.Name, rel.Branch, img.Tags, rel.Channel))
	}
	attempts := 1
	if len(img.ComputedArgs) > 0 {
		attempts = 2
	}
	var buildErr error
	for res.Attempts < attempts {
		res.Attempts++
		args, err := d.buildArgs(ctx, img)
		if err == nil {
			_, err = d.docker(ctx, nil, d.buildArgv(img, args, res.Refs)...)
		}
		if buildErr = err; err == nil || ctx.Err() != nil {
			break
		}
		if res.Attempts < attempts {
			fmt.Fprintf(d.Output, "build of %s failed (%v); retrying once with the computed arguments computed again\n", img.Name, err)
		}
	}
	if buildErr != nil {
		return fail(fmt.Errorf("build %s: %w", img.Name, buildErr))
	}
	if d.DryRun {
		res.Status = "built"
		return res
	}
	for _, ref := range res.Refs {
		out, err := d.docker(ctx, nil, "push", ref)
		if err != nil {
			return fail(fmt.Errorf("push %s: %w", ref, err))
		}
		if m := digestRe.FindSubmatch(out); m != nil && res.Digest == "" {
			res.Digest = string(m[1])
		}
	}
	res.Status = "pushed"
	return res
}

// buildArgs are the image's build arguments: build_args with ${version},
// ${channel} and ${tag} substituted, and computed_args run in the clone
// (their trimmed stdout), sorted by name.
func (d Docker) buildArgs(ctx context.Context, img pipelineconfig.Image) ([]string, error) {
	rel := d.Release
	sub := strings.NewReplacer("${version}", rel.Version, "${channel}", rel.Channel, "${tag}", rel.GitTag)
	values := map[string]string{}
	for k, v := range img.BuildArgs {
		values[k] = sub.Replace(v)
	}
	for _, k := range slices.Sorted(maps.Keys(img.ComputedArgs)) {
		arg := img.ComputedArgs[k]
		out, err := d.Args.Capture(ctx, shiprun.Step{Name: "docker", Stage: "docker", Command: shipconfig.Command{Run: arg.Run, Shell: arg.Shell}}, shiprun.Vars{
			Project: "pipeline", Version: rel.Version, Channel: rel.Channel, Tag: rel.GitTag, GitHead: d.Revision, Branch: rel.Branch, DryRun: d.DryRun,
		})
		if err != nil {
			return nil, fmt.Errorf("computing %s: %w", k, err)
		}
		values[k] = strings.TrimSpace(out)
	}
	var out []string
	for _, k := range slices.Sorted(maps.Keys(values)) {
		out = append(out, k+"="+values[k])
	}
	return out, nil
}

// buildArgv is `docker build --platform ... -f ... --build-arg ... --label
// ... -t ... <context>` (BuildKit, the default builder of Docker Engine 23+,
// takes --platform without buildx).
func (d Docker) buildArgv(img pipelineconfig.Image, args, refs []string) []string {
	argv := []string{"build", "--platform", img.Platform, "-f", img.Dockerfile}
	for _, a := range args {
		argv = append(argv, "--build-arg", a)
	}
	labels := []string{
		"org.opencontainers.image.title=" + img.Title,
		"org.opencontainers.image.version=" + d.Release.Version,
		"org.opencontainers.image.revision=" + d.Revision,
	}
	if d.Source != "" {
		labels = append(labels, "org.opencontainers.image.source="+d.Source)
	}
	for _, l := range labels {
		argv = append(argv, "--label", l)
	}
	for _, ref := range refs {
		argv = append(argv, "-t", ref)
	}
	return append(argv, img.Context)
}

// docker runs one docker command in the clone, streaming its output, and
// returns the end of that output.
func (d Docker) docker(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	env := d.Env
	if env == nil {
		env = os.Environ()
	}
	// The token reaches docker only on login's stdin: no child gets the
	// registry's or the deploy's secrets in its environment.
	env = SafeEnv(env, d.Config.Registry)
	res, err := d.Proc.Run(ctx, proc.Command{Name: "docker", Args: args, Dir: d.Dir, Env: env, Stdin: stdin, Stdout: d.Output, Stderr: d.Output})
	if err != nil {
		if tail := lastOutputLines(res.Stdout, 10); tail != "" {
			err = fmt.Errorf("%w\nlast output:\n%s", err, tail)
		}
	}
	return res.Stdout, err
}

// lastOutputLines returns the last n non-empty lines of a command's
// (combined) output.
func lastOutputLines(out []byte, n int) string {
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimRight(l, "\r "); strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
