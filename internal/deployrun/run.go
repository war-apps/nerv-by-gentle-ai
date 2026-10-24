// Package deployrun runs `nerv deploy`: it takes an artifact `nerv ship`
// left, makes the release on origin and deploys it.
//
//  1. verify-artifact: the artifact's digests, and nothing unlisted in it;
//  2. clone: a throwaway clone of the remote's branch, with its tags and
//     semantic-release notes;
//  3. commit: the release commit, the artifact's release files on its base
//     commit with deploy.git.commit_message (when origin's branch moved
//     past the base, only with Rebase, cherry-picked on the new head);
//  4. tag: the annotated tag;
//  5. notes: semantic-release's channel note refs/notes/semantic-release-
//     <tag>, {"channels":[<channel or null>]}, on the release commit (where
//     git log --tags reads it, as the version engine does);
//  6. push: the branch, the tag and the note, atomically, never forced;
//  7. publish:<target> for every target, then deploy:<target> and
//     verify:<target> for each, in deploy.targets order;
//  8. release: the git host's release (a prerelease on a channel), unless
//     it exists.
//
// A re-run is idempotent: a tag already on origin whose commit holds the
// same release is reused, as is a release commit already on the branch;
// only what is missing is pushed, and an existing release is skipped. The
// report says, step by step, what was done.
package deployrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/deployconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/deployprovider"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
)

// Options configure one deploy.
type Options struct {
	// RepoRoot is the user's repository, whose Config.Git.Remote is cloned.
	RepoRoot string
	// ArtifactDir is the artifact to deploy.
	ArtifactDir string
	Config      *deployconfig.Deploy
	// Targets restricts the deploy to these targets (in config order);
	// empty means every target.
	Targets []string
	// DryRun prepares the release in the clone and stops before the push.
	DryRun bool
	// Rebase re-applies the release on origin's branch when it moved past
	// the artifact's base commit.
	Rebase bool
	// Confirm is asked after the release is prepared, before anything is
	// pushed or run; an error stops the deploy. Nil proceeds.
	Confirm func(Report) error
	// Providers holds the provider plugins: deploy-<name>/deploy-provider.yaml.
	Providers fs.FS
	// UserValue returns a provider config key's user-scope value
	// (deploy.providers.<provider>.<key>); nil has none.
	UserValue func(provider, key string) (string, bool)
	// Proc runs the providers' commands and Git runs git; nil means
	// proc.ExecRunner.
	Proc, Git proc.Runner
	// LookPath checks the providers' requires; nil means exec.LookPath.
	LookPath func(string) (string, error)
	// Nerv is {nerv}; empty means os.Executable.
	Nerv string
	// ProcEnv is the providers' and git's environment; nil inherits.
	ProcEnv []string
	// Output receives progress and the providers' stderr; nil discards.
	Output io.Writer
	// TempDir holds the clone; empty means os.TempDir.
	TempDir string
}

// provider is a loaded provider with its resolved config.
type provider struct {
	manifest *deployprovider.Manifest
	config   map[string]string
	target   string
}

func refuse(format string, args ...any) error {
	return &refusal.Error{Err: fmt.Errorf(format, args...)}
}

// Run deploys the artifact. The report is filled as far as the deploy went,
// also on error; a refusal (refusal.Is) means a precondition failed and
// nothing was written.
func Run(ctx context.Context, o Options) (Report, error) {
	if o.Output == nil {
		o.Output = io.Discard
	}
	if o.Proc == nil {
		o.Proc = proc.ExecRunner{}
	}
	if o.Git == nil {
		o.Git = proc.ExecRunner{}
	}
	if o.LookPath == nil {
		o.LookPath = exec.LookPath
	}
	rep := Report{DryRun: o.DryRun}
	targets, err := selectTargets(o.Config, o.Targets)
	if err != nil {
		return rep, err
	}
	for _, name := range []string{"verify-artifact", "clone", "commit", "tag", "notes", "push"} {
		rep.Steps = append(rep.Steps, Step{Name: name, Status: StatusNotRun})
	}
	for _, op := range []string{deployprovider.OpPublish, deployprovider.OpDeploy, deployprovider.OpVerify} {
		for _, t := range targets {
			rep.Steps = append(rep.Steps, Step{Name: op + ":" + t.Name, Status: StatusNotRun})
		}
	}
	rep.Steps = append(rep.Steps, Step{Name: "release", Status: StatusNotRun})
	tr := tracker{report: &rep, out: o.Output}
	d := &deployer{o: o, tr: tr, rep: &rep}
	return rep, d.run(ctx, targets)
}

// selectTargets returns the configured targets, restricted to names.
func selectTargets(cfg *deployconfig.Deploy, names []string) ([]deployconfig.Target, error) {
	if len(names) == 0 {
		return cfg.Targets, nil
	}
	var known []string
	for _, t := range cfg.Targets {
		known = append(known, t.Name)
	}
	for _, n := range names {
		if !slices.Contains(known, n) {
			return nil, refuse("--target %s: no such target in deploy.targets (%s)", n, orNone(strings.Join(known, ", ")))
		}
	}
	var out []deployconfig.Target
	for _, t := range cfg.Targets {
		if slices.Contains(names, t.Name) {
			out = append(out, t)
		}
	}
	return out, nil
}

type deployer struct {
	o   Options
	tr  tracker
	rep *Report
	m   *artifact.Manifest
	ws  workspace
}

func (d *deployer) run(ctx context.Context, targets []deployconfig.Target) error {
	o, tr := d.o, d.tr
	m, err := artifact.Verify(o.ArtifactDir)
	if err != nil {
		tr.set("verify-artifact", StatusFailed, "%v", err)
		return &refusal.Error{Err: err}
	}
	d.m = m
	d.rep.Project, d.rep.Version, d.rep.Tag, d.rep.Channel, d.rep.Branch = m.Project, m.Version, m.Tag, m.Channel, m.Branch
	tr.set("verify-artifact", StatusDone, "%s %s (%s)", m.Project, m.Version, m.Tag)

	host, provs, err := d.loadProviders(targets)
	if err != nil {
		return err
	}
	notes, err := os.ReadFile(filepath.Join(o.ArtifactDir, artifact.NotesFile))
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(notes)) == "" {
		fmt.Fprintf(o.Output, "nerv deploy: warning: %s has empty release notes (shipped with --version?): the commit and the release get none\n", m.Version)
	}
	url, err := d.remoteURL(ctx)
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp(o.TempDir, "nerv-deploy-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	d.ws = workspace{runner: o.Git, dir: filepath.Join(tmp, "clone"), env: o.ProcEnv}
	if err := d.ws.clone(ctx, url, m.Branch); err != nil {
		tr.set("clone", StatusFailed, "%v", err)
		return err
	}
	tr.set("clone", StatusDone, "%s %s", o.Config.Git.Remote, m.Branch)

	message := deployconfig.CommitMessage(o.Config.Git.CommitMessage, deployconfig.Release{Version: m.Version, Tag: m.Tag, Channel: m.Channel, Notes: string(notes)})
	refs, err := d.prepare(ctx, message)
	if err != nil {
		return err
	}

	if o.DryRun {
		tr.rest(StatusDryRun, "")
		return nil
	}
	if o.Confirm != nil {
		if err := o.Confirm(*d.rep); err != nil {
			return err
		}
	}
	if len(refs) == 0 {
		tr.set("push", StatusSkipped, "origin has the release commit, the tag and the note")
	} else {
		args := append([]string{"-c", "credential.interactive=never", "push", "--atomic", "--quiet", "origin"}, refs...)
		if _, err := d.ws.git(ctx, args...); err != nil {
			tr.set("push", StatusFailed, "%v", err)
			return err
		}
		tr.set("push", StatusDone, "%s", strings.Join(refs, " "))
	}
	return d.runProviders(ctx, host, provs)
}

// remoteURL is the user's remote's URL.
func (d *deployer) remoteURL(ctx context.Context) (string, error) {
	remote := d.o.Config.Git.Remote
	res, err := d.o.Git.Run(ctx, proc.Command{Name: "git", Args: []string{"-C", d.o.RepoRoot, "remote", "get-url", "--", remote}, Env: d.o.ProcEnv})
	url := strings.TrimSpace(string(res.Stdout))
	if err != nil || url == "" {
		return "", refuse("the repository has no remote %q (deploy.git.remote) to release to", remote)
	}
	return url, nil
}

// prepare makes the release commit, the tag and the note in the clone, as
// needed, and returns the refs to push.
func (d *deployer) prepare(ctx context.Context, message string) ([]string, error) {
	w, m, tr := d.ws, d.m, d.tr
	files := m.ReleaseFiles
	if w.resolve(ctx, m.BaseCommit) == "" {
		return nil, refuse("the artifact's base commit %s is not on %s: it was built from a commit origin does not have", m.BaseCommit, d.o.Config.Git.Remote)
	}
	head := w.resolve(ctx, "refs/remotes/origin/"+m.Branch)
	if head == "" {
		return nil, refuse("origin has no branch %s", m.Branch)
	}
	var refs []string
	var commit string
	if tagged := w.resolve(ctx, "refs/tags/"+m.Tag); tagged != "" {
		if !w.matches(ctx, tagged, m.BaseCommit, message, files) {
			return nil, refuse("the tag %s already exists on origin with other content (commit %s): it is another release, not this artifact's", m.Tag, short(tagged))
		}
		commit = tagged
		tr.set("commit", StatusSkipped, "already on origin (%s)", short(commit))
		tr.set("tag", StatusSkipped, "already on origin")
		switch {
		case w.ok(ctx, "merge-base", "--is-ancestor", commit, head):
		case w.ok(ctx, "merge-base", "--is-ancestor", head, commit):
			refs = append(refs, commit+":refs/heads/"+m.Branch)
		default:
			return nil, refuse("origin's %s no longer contains %s's release commit %s", m.Branch, m.Tag, short(commit))
		}
	} else {
		var err error
		if commit, err = d.releaseCommit(ctx, head, message); err != nil {
			return nil, err
		}
		if commit != head {
			refs = append(refs, commit+":refs/heads/"+m.Branch)
		}
		if _, err := w.git(ctx, "tag", "--annotate", "--message", m.Tag, "--", m.Tag, commit); err != nil {
			tr.set("tag", StatusFailed, "%v", err)
			return nil, err
		}
		refs = append(refs, "refs/tags/"+m.Tag)
		tr.set("tag", StatusDone, "%s on %s", m.Tag, short(commit))
	}
	d.rep.Commit = commit

	note := channelNote(m.Channel)
	ref := strings.TrimPrefix(notesRef(m.Tag), "refs/notes/")
	if existing, err := w.git(ctx, "notes", "--ref", ref, "show", commit); err == nil && existing == note {
		tr.set("notes", StatusSkipped, "already on origin")
	} else {
		if _, err := w.git(ctx, "notes", "--ref", ref, "add", "--force", "--message", note, commit); err != nil {
			tr.set("notes", StatusFailed, "%v", err)
			return nil, err
		}
		refs = append(refs, notesRef(m.Tag)+":"+notesRef(m.Tag))
		tr.set("notes", StatusDone, "%s %s", notesRef(m.Tag), note)
	}
	return refs, nil
}

// releaseCommit returns the release commit to tag: a new one on the base,
// the one already on origin's branch, or, with Rebase, a new one
// cherry-picked on origin's moved head.
func (d *deployer) releaseCommit(ctx context.Context, head, message string) (string, error) {
	w, m, tr := d.ws, d.m, d.tr
	base, files := m.BaseCommit, m.ReleaseFiles
	if !w.ok(ctx, "merge-base", "--is-ancestor", base, head) {
		return "", refuse("origin's %s no longer contains the artifact's base commit %s (was it rewritten?): ship again", m.Branch, short(base))
	}
	if head != base && w.matches(ctx, head, base, message, files) {
		tr.set("commit", StatusSkipped, "already on origin (%s)", short(head))
		return head, nil
	}
	if head != base && !d.o.Rebase {
		n, _ := w.git(ctx, "rev-list", "--count", base+".."+head)
		return "", refuse("origin's %s moved past the artifact's base commit %s (%s new commits): pass --rebase to re-apply the release files on its new head, or ship again", m.Branch, short(base), n)
	}
	commit, made, err := w.commitRelease(ctx, base, d.o.ArtifactDir, message, files)
	if err != nil {
		tr.set("commit", StatusFailed, "%v", err)
		return "", err
	}
	if head != base {
		if made {
			commit, err = w.rebase(ctx, commit, head)
		} else if !w.hasFiles(ctx, head, files) {
			commit, made, err = w.commitRelease(ctx, head, d.o.ArtifactDir, message, files)
		} else {
			commit = head
		}
		if errors.Is(err, errConflict) {
			tr.set("commit", StatusFailed, "the release files conflict with origin's new commits")
			return "", refuse("the release files do not apply on origin's new %s head %s (conflict): ship again from it", m.Branch, short(head))
		}
		if err != nil {
			tr.set("commit", StatusFailed, "%v", err)
			return "", err
		}
	}
	switch {
	case !made:
		tr.set("commit", StatusSkipped, "the release changes no file: %s is tagged", short(commit))
	case head != base:
		tr.set("commit", StatusDone, "%s, rebased on %s", short(commit), short(head))
	default:
		tr.set("commit", StatusDone, "%s on %s", short(commit), short(base))
	}
	return commit, nil
}

// loadProviders loads the host and the targets' providers, checks their
// kind, resolves their config and checks that what they require is on PATH.
func (d *deployer) loadProviders(targets []deployconfig.Target) (*provider, []provider, error) {
	var host *provider
	var tools []string
	if name := d.o.Config.Host; name != "" {
		m, err := d.manifest(name, deployprovider.KindGitHost, "deploy.host")
		if err != nil {
			return nil, nil, err
		}
		cfg, err := d.resolveConfig(m, nil, "deploy.host")
		if err != nil {
			return nil, nil, err
		}
		host = &provider{manifest: m, config: cfg}
		tools = append(tools, m.Requires...)
	}
	var provs []provider
	for i, t := range targets {
		at := fmt.Sprintf("deploy.targets[%d] (%s)", i, t.Name)
		m, err := d.manifest(t.Provider, deployprovider.KindTarget, at)
		if err != nil {
			return nil, nil, err
		}
		cfg, err := d.resolveConfig(m, t.Config, at)
		if err != nil {
			return nil, nil, err
		}
		provs = append(provs, provider{manifest: m, config: cfg, target: t.Name})
		tools = append(tools, m.Requires...)
	}
	var missing []string
	for _, tool := range tools {
		if _, err := d.o.LookPath(tool); err != nil && !slices.Contains(missing, tool) {
			missing = append(missing, tool)
		}
	}
	if len(missing) > 0 {
		return nil, nil, refuse("%s is not on PATH: the deploy providers need it", strings.Join(missing, ", "))
	}
	return host, provs, nil
}

func (d *deployer) manifest(name, kind, at string) (*deployprovider.Manifest, error) {
	if d.o.Providers == nil {
		return nil, refuse("%s: no deploy provider is available", at)
	}
	data, err := fs.ReadFile(d.o.Providers, "deploy-"+name+"/"+deployprovider.ManifestFile)
	if err != nil {
		return nil, refuse("%s: deploy provider %q is not available (no deploy-%s plugin)", at, name, name)
	}
	m, err := deployprovider.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("deploy-%s: %w", name, err)
	}
	if m.Kind != kind {
		return nil, refuse("%s: provider %s is a %s provider, not a %s", at, name, m.Kind, kind)
	}
	return m, nil
}

// resolveConfig gives each declared key its value: the target's, the user
// scope's, or the default. An undeclared or ill-typed key is refused.
func (d *deployer) resolveConfig(m *deployprovider.Manifest, given map[string]string, at string) (map[string]string, error) {
	var problems []string
	for _, key := range sortedKeys(given) {
		if _, ok := m.Key(key); !ok {
			var keys []string
			for _, c := range m.Config {
				keys = append(keys, c.Key)
			}
			problems = append(problems, fmt.Sprintf("config.%s is not a key of provider %s (%s)", key, m.Name, orNone(strings.Join(keys, ", "))))
		}
	}
	cfg := map[string]string{}
	for _, c := range m.Config {
		v, ok := given[c.Key]
		if !ok && d.o.UserValue != nil {
			v, ok = d.o.UserValue(m.Name, c.Key)
		}
		if !ok {
			v = c.Default
		}
		if v != "" && !deployprovider.ValidValue(c.Type, v) {
			problems = append(problems, fmt.Sprintf("config.%s: %q is not a valid %s", c.Key, v, c.Type))
		}
		cfg[c.Key] = v
	}
	if len(problems) > 0 {
		return nil, refuse("%s: %s", at, strings.Join(problems, "; "))
	}
	return cfg, nil
}

// env is what p's operations run with.
func (d *deployer) env(p provider) (deployprovider.Env, error) {
	m, dir := d.m, d.o.ArtifactDir
	nerv := d.o.Nerv
	if nerv == "" {
		var err error
		if nerv, err = os.Executable(); err != nil {
			return deployprovider.Env{}, err
		}
	}
	prerelease := "false"
	if m.Channel != "" {
		prerelease = "true"
	}
	values := map[string]string{
		"project": m.Project, "version": m.Version, "channel": m.Channel, "tag": m.Tag, "base_commit": m.BaseCommit,
		"branch": m.Branch, "build_mode": m.BuildMode, "prerelease": prerelease, "artifact_dir": dir,
		"notes_file": filepath.Join(dir, artifact.NotesFile), "changelog_file": filepath.Join(dir, artifact.ChangelogFile),
		"target": p.target, "nerv": nerv,
	}
	for k, v := range p.config {
		values["config."+k] = v
	}
	lists := map[string][]string{"images": append([]string{}, m.Images...)}
	for _, out := range m.Outputs {
		abs := filepath.Join(dir, filepath.FromSlash(out.Path))
		lists["outputs"] = append(lists["outputs"], abs)
		if out.Kind == artifact.KindImageTar {
			lists["image_tars"] = append(lists["image_tars"], abs)
		}
	}
	return deployprovider.Env{
		Values: values, Lists: lists, Dir: d.ws.dir, ProcEnv: d.o.ProcEnv, Output: d.o.Output,
		Request: deployprovider.Request{Target: p.target, Artifact: m, ArtifactDir: dir, Config: p.config},
	}, nil
}

// runProviders runs every target's publish, then each target's deploy and
// verify, then the git host's release, stopping at the first failure.
func (d *deployer) runProviders(ctx context.Context, host *provider, provs []provider) error {
	tr := d.tr
	run := func(p provider, op string) error {
		step := op + ":" + p.target
		if !p.manifest.Implements(op) {
			tr.set(step, StatusSkipped, "provider %s does not implement %s", p.manifest.Name, op)
			return nil
		}
		env, err := d.env(p)
		if err != nil {
			return err
		}
		out, err := p.manifest.Run(ctx, d.o.Proc, op, env)
		switch {
		case err != nil:
			tr.set(step, StatusFailed, "%v", err)
			return err
		case out.Status == deployprovider.StatusSkipped:
			tr.set(step, StatusSkipped, "%s", out.Message)
		default:
			tr.set(step, StatusDone, "%s", out.Message)
		}
		return nil
	}
	for _, p := range provs {
		if err := run(p, deployprovider.OpPublish); err != nil {
			return err
		}
	}
	for _, p := range provs {
		for _, op := range []string{deployprovider.OpDeploy, deployprovider.OpVerify} {
			if err := run(p, op); err != nil {
				return err
			}
		}
	}
	if host == nil {
		tr.set("release", StatusSkipped, "deploy.host is not set")
		return nil
	}
	env, err := d.env(*host)
	if err != nil {
		return err
	}
	exists, err := host.manifest.ReleaseExists(ctx, d.o.Proc, env)
	if err != nil {
		tr.set("release", StatusFailed, "%v", err)
		return err
	}
	if exists {
		tr.set("release", StatusSkipped, "the %s release %s exists", host.manifest.Name, d.m.Tag)
		return nil
	}
	if _, err := host.manifest.Run(ctx, d.o.Proc, deployprovider.OpCreateRelease, env); err != nil {
		tr.set("release", StatusFailed, "%v", err)
		return err
	}
	kind := "release"
	if d.m.Channel != "" {
		kind = "prerelease"
	}
	tr.set("release", StatusDone, "%s %s %s", host.manifest.Name, kind, d.m.Tag)
	return nil
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
