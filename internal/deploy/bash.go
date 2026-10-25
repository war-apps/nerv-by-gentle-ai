package deploy

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// BashDeps are the effects of the deploy-bash operations.
type BashDeps struct {
	// Proc runs git and the scripts; nil means proc.ExecRunner.
	Proc proc.Runner
	// Dir is where the operation runs: the user's repository for
	// preflight, nerv deploy's clone of the release for the others; empty
	// means the current directory.
	Dir string
	// Env is the scripts' and git's environment, before the deploy
	// variables; nil means os.Environ.
	Env []string
	// Output receives the scripts' stderr live; nil discards it.
	Output io.Writer
	// TempDir holds the release commit's tree while a script runs; empty
	// means os.TempDir.
	TempDir string
}

// The deploy-bash config keys, one per operation that runs a script.
var bashScriptKeys = map[string]string{
	opPublish: "publish_script",
	opDeploy:  "script",
	opVerify:  "verify_script",
}

// opPublish is the deploy-bash operation that runs publish_script.
const opPublish = "publish"

// bashPayload is the part of deploy's request payload deploy-bash reads.
type bashPayload struct {
	Target      string `json:"target"`
	ArtifactDir string `json:"artifact_dir"`
	Artifact    struct {
		Project       string   `json:"project"`
		Version       string   `json:"version"`
		Channel       string   `json:"channel"`
		Tag           string   `json:"tag"`
		BaseCommit    string   `json:"base_commit"`
		Branch        string   `json:"branch"`
		BuildProvider string   `json:"build_provider"`
		Images        []string `json:"images"`
	} `json:"artifact"`
}

// RunBashOp is `nerv provider deploy-bash <op>`, the native operations of
// the deploy-bash target, which runs the project's own deploy scripts. It
// reads the request on stdin and prints one JSON response (the common
// envelope); it returns 0 (ok or skipped) or 1.
//
//   - preflight runs in the user's repository before nerv deploy writes
//     anything: config.script, and publish_script and verify_script when
//     set, must each be a relative path to a regular file inside the
//     repository (no .., no absolute path, outside .git, no symlink
//     leading out of it).
//   - publish, deploy and verify run in nerv deploy's clone: the script
//     (publish_script, script, verify_script) must be a regular file of the
//     release commit (the artifact's tag), never a symlink; that commit's
//     tree is extracted into a temporary directory and the script runs
//     there with bash, the request on its stdin and the deploy variables
//     (BashVariables) in its environment. The script's JSON response, when
//     it prints one, is the operation's; otherwise its exit code answers.
//     publish and verify without their script are skipped.
func RunBashOp(ctx context.Context, op string, stdin io.Reader, stdout io.Writer, deps BashDeps) int {
	return extension.Answer(stdout, bashOp(ctx, op, stdin, deps))
}

func bashOp(ctx context.Context, op string, stdin io.Reader, deps BashDeps) extension.Response {
	fail := func(format string, args ...any) extension.Response {
		return extension.Response{Status: extension.StatusFailed, Message: fmt.Sprintf(format, args...)}
	}
	if _, ok := bashScriptKeys[op]; !ok && op != opPreflight {
		return fail("unknown operation %q (want preflight, publish, deploy or verify)", op)
	}
	req, err := extension.ReadRequest(stdin)
	if err != nil {
		return fail("reading the request on stdin: %v", err)
	}
	if req.Operation != op {
		return fail("the request is for operation %q, not %s", req.Operation, op)
	}
	var cfg map[string]string
	if err := req.DecodeConfig(&cfg); err != nil {
		return fail("config: %v", err)
	}
	dir := deps.Dir
	if dir == "" {
		if dir, err = os.Getwd(); err != nil {
			return fail("%v", err)
		}
	}
	if op == opPreflight {
		var payload bashPayload
		if err := req.DecodePayload(&payload); err != nil {
			return fail("payload: %v", err)
		}
		git := bashGit(ctx, dir, deps)
		base := payload.Artifact.BaseCommit
		res, err := git(nil, "rev-parse", "--verify", "--quiet", base+"^{commit}")
		// A base commit the repository lacks leaves the commit check to
		// deploy, which runs it on the release commit in the clone.
		haveBase := base != "" && err == nil && strings.TrimSpace(string(res.Stdout)) != ""
		label := "the base commit " + shortCommit(base)
		for _, key := range []string{"script", "publish_script", "verify_script"} {
			if cfg[key] == "" && key != "script" {
				continue
			}
			if err := checkWorkingScript(dir, key, cfg[key]); err != nil {
				return fail("%v", err)
			}
			if !haveBase {
				continue
			}
			clean, _ := scriptPath(key, cfg[key])
			if err := checkCommitScript(git, base, label, key, cfg[key], clean); err != nil {
				return fail("%v", err)
			}
		}
		return extension.Response{Status: extension.StatusOK}
	}
	key := bashScriptKeys[op]
	if cfg[key] == "" {
		if op == opDeploy {
			return fail("config.script is required")
		}
		return extension.Response{Status: extension.StatusSkipped, Message: fmt.Sprintf("config.%s is not set", key)}
	}
	var payload bashPayload
	if err := req.DecodePayload(&payload); err != nil {
		return fail("payload: %v", err)
	}
	return runBashScript(ctx, req, payload, key, cfg[key], dir, deps)
}

// scriptPath returns the clean slash-separated form of a configured script
// path, refusing one that is absolute, climbs out with .., or lies under
// .git.
func scriptPath(key, raw string) (string, error) {
	clean := path.Clean(filepath.ToSlash(raw))
	bad := raw == "" || strings.ContainsAny(raw, "\x00\r\n") || !filepath.IsLocal(filepath.FromSlash(clean))
	for _, part := range strings.Split(clean, "/") {
		bad = bad || strings.EqualFold(part, ".git")
	}
	if bad {
		return "", fmt.Errorf("config.%s %q must be a relative path inside the repository (no .., not absolute, outside .git)", key, raw)
	}
	return clean, nil
}

// checkWorkingScript checks a configured script in the repository's
// working tree at root.
func checkWorkingScript(root, key, raw string) error {
	if raw == "" {
		return fmt.Errorf("config.%s is required: the repository's deploy script, relative to its root", key)
	}
	clean, err := scriptPath(key, raw)
	if err != nil {
		return err
	}
	file := filepath.Join(root, filepath.FromSlash(clean))
	info, err := os.Stat(file)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("config.%s %q: no such file in the repository", key, raw)
	}
	if err != nil {
		return fmt.Errorf("config.%s %q: %v", key, raw, err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(file)
	if err != nil {
		return err
	}
	if rel, err := filepath.Rel(realRoot, real); err != nil || (rel != "." && !filepath.IsLocal(rel)) {
		return fmt.Errorf("config.%s %q resolves outside the repository (%s)", key, raw, real)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("config.%s %q is not a regular file", key, raw)
	}
	return nil
}

// bashGit runs git in dir, with pathspecs taken literally.
func bashGit(ctx context.Context, dir string, deps BashDeps) func(io.Writer, ...string) (proc.Result, error) {
	runner := deps.Proc
	if runner == nil {
		runner = proc.ExecRunner{}
	}
	env := deps.Env
	if env == nil {
		env = os.Environ()
	}
	return func(stdout io.Writer, args ...string) (proc.Result, error) {
		return runner.Run(ctx, proc.Command{Name: "git", Args: append([]string{"-C", dir, "--literal-pathspecs"}, args...), Env: env, Stdout: stdout})
	}
}

// checkCommitScript checks that the configured script clean is a regular
// file (not a symlink) in commit, named by label in the refusal.
func checkCommitScript(git func(io.Writer, ...string) (proc.Result, error), commit, label, key, raw, clean string) error {
	res, err := git(nil, "ls-tree", "-z", commit, "--", clean)
	if err != nil {
		return fmt.Errorf("git ls-tree %s: %v: %s", label, err, extension.LastLine(res.Stderr))
	}
	mode, kind, ok := treeEntry(res.Stdout, clean)
	switch {
	case !ok:
		return fmt.Errorf("config.%s %q is not in %s (commit it before releasing)", key, raw, label)
	case mode == "120000":
		return fmt.Errorf("config.%s %q is a symlink in %s: name the file itself", key, raw, label)
	case kind != "blob":
		return fmt.Errorf("config.%s %q is not a regular file in %s", key, raw, label)
	}
	return nil
}

// shortCommit is the first 12 characters of commit.
func shortCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

// runBashScript runs the release commit's script in a copy of that
// commit's tree.
func runBashScript(ctx context.Context, req extension.Request, p bashPayload, key, raw, dir string, deps BashDeps) extension.Response {
	fail := func(format string, args ...any) extension.Response {
		return extension.Response{Status: extension.StatusFailed, Message: fmt.Sprintf(format, args...)}
	}
	runner := deps.Proc
	if runner == nil {
		runner = proc.ExecRunner{}
	}
	env := deps.Env
	if env == nil {
		env = os.Environ()
	}
	clean, err := scriptPath(key, raw)
	if err != nil {
		return fail("%v", err)
	}
	tag := p.Artifact.Tag
	git := bashGit(ctx, dir, deps)
	res, err := git(nil, "rev-parse", "--verify", "--quiet", "refs/tags/"+tag+"^{commit}")
	commit := strings.TrimSpace(string(res.Stdout))
	if tag == "" || err != nil || commit == "" {
		return fail("the release tag %s is not in %s: deploy-bash runs in nerv deploy's clone of the release", orDash(tag), dir)
	}
	if err := checkCommitScript(git, commit, "the release commit "+tag, key, raw, clean); err != nil {
		return fail("%v", err)
	}

	tmp, err := os.MkdirTemp(deps.TempDir, "nerv-deploy-bash-")
	if err != nil {
		return fail("%v", err)
	}
	defer os.RemoveAll(tmp)
	tree := filepath.Join(tmp, "tree")
	if err := extractCommit(git, commit, tmp, tree); err != nil {
		return fail("extracting the release commit %s: %v", tag, err)
	}

	call := extension.Call{Argv: []string{"bash", clean}, Dir: tree, Env: withVariables(env, bashVariables(req.Operation, p, commit)), Stderr: deps.Output}
	resp, _, err := extension.Exec(ctx, runner, "script "+clean, call, req)
	switch {
	case err != nil && resp.Failure():
		return extension.Response{Status: resp.Status, Message: clean + ": " + resp.Message, Data: resp.Data}
	case err != nil:
		return fail("%v", err)
	}
	if resp.Status == "" {
		resp.Status = extension.StatusOK
	}
	if resp.Message == "" {
		resp.Message = clean + " done"
	}
	return resp
}

// treeEntry reads the mode and type of name in git ls-tree -z output.
func treeEntry(out []byte, name string) (mode, kind string, ok bool) {
	for _, entry := range bytes.Split(out, []byte{0}) {
		meta, entryName, found := strings.Cut(string(entry), "\t")
		fields := strings.Fields(meta)
		if found && entryName == name && len(fields) == 3 {
			return fields[0], fields[1], true
		}
	}
	return "", "", false
}

// extractCommit writes commit's tree into tree, through git archive. A
// symlink is made after every file, so no file is written through one,
// and an entry leaving tree is refused.
func extractCommit(git func(io.Writer, ...string) (proc.Result, error), commit, tmp, tree string) error {
	archive := filepath.Join(tmp, "tree.tar")
	f, err := os.Create(archive)
	if err != nil {
		return err
	}
	res, err := git(f, "archive", "--format=tar", commit)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("git archive: %v: %s", err, extension.LastLine(res.Stderr))
	}
	f, err = os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := os.Mkdir(tree, 0o700); err != nil {
		return err
	}
	type link struct{ name, target string }
	var links []link
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name := path.Clean(h.Name)
		if !filepath.IsLocal(filepath.FromSlash(name)) {
			return fmt.Errorf("archive entry %q leaves the tree", h.Name)
		}
		dest := filepath.Join(tree, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeTreeFile(dest, tr, os.FileMode(h.Mode)&0o755|0o600); err != nil {
				return err
			}
		case tar.TypeSymlink:
			links = append(links, link{dest, h.Linkname})
		}
	}
	for _, l := range links {
		if err := os.MkdirAll(filepath.Dir(l.name), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(l.target, l.name); err != nil {
			return err
		}
	}
	return nil
}

func writeTreeFile(dest string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// bashVariables are the deploy variables a deploy-bash script finds in its
// environment, besides the request on its stdin: NERV_DEPLOY_OPERATION,
// _TARGET, _PROJECT, _VERSION, _TAG, _CHANNEL (empty for a release),
// _PRERELEASE (true on a channel), _BRANCH, _BASE_COMMIT, _COMMIT (the
// release commit, the tag's), _BUILD_PROVIDER, _ARTIFACT_DIR, _NOTES_FILE,
// _CHANGELOG_FILE and _IMAGES (the image references, space-separated).
func bashVariables(op string, p bashPayload, commit string) map[string]string {
	a := p.Artifact
	prerelease := "false"
	if a.Channel != "" {
		prerelease = "true"
	}
	return map[string]string{
		"NERV_DEPLOY_OPERATION": op, "NERV_DEPLOY_TARGET": p.Target, "NERV_DEPLOY_PROJECT": a.Project,
		"NERV_DEPLOY_VERSION": a.Version, "NERV_DEPLOY_TAG": a.Tag, "NERV_DEPLOY_CHANNEL": a.Channel,
		"NERV_DEPLOY_PRERELEASE": prerelease, "NERV_DEPLOY_BRANCH": a.Branch, "NERV_DEPLOY_BASE_COMMIT": a.BaseCommit,
		"NERV_DEPLOY_COMMIT": commit, "NERV_DEPLOY_BUILD_PROVIDER": a.BuildProvider, "NERV_DEPLOY_ARTIFACT_DIR": p.ArtifactDir,
		"NERV_DEPLOY_NOTES_FILE":     filepath.Join(p.ArtifactDir, artifact.NotesFile),
		"NERV_DEPLOY_CHANGELOG_FILE": filepath.Join(p.ArtifactDir, artifact.ChangelogFile),
		"NERV_DEPLOY_IMAGES":         strings.Join(a.Images, " "),
	}
}

// withVariables is env with vars set, replacing any value env had for
// them.
func withVariables(env []string, vars map[string]string) []string {
	out := make([]string, 0, len(env)+len(vars))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if _, ok := vars[name]; !ok {
			out = append(out, kv)
		}
	}
	for _, name := range sortedNames(vars) {
		out = append(out, name+"="+vars[name])
	}
	return out
}

func sortedNames(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	slices.Sort(names)
	return names
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
