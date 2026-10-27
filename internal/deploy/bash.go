package deploy

import (
	"bufio"
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
	"strconv"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/artifact"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
)

// BashDeps are the effects of the release-script operations.
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

// The release-script config keys, one per operation that runs a script.
var bashScriptKeys = map[string]string{
	opPublish: "publish_script",
	opDeploy:  "script",
	opVerify:  "verify_script",
}

// opPublish is the release-script operation that runs publish_script.
const opPublish = "publish"

// bashPayload is the part of deploy's request payload release-script reads.
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

// RunBashOp is `nerv provider release-script <op>`, the native operations of
// the release-script target, which runs the project's own deploy scripts. It
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
//     whole tree (export-ignore attributes do not apply) is extracted into
//     a temporary directory and the script runs there with bash, the
//     request on its stdin and the deploy variables (BashVariables) in its
//     environment. The script's JSON response, when it prints one, is the
//     operation's; otherwise its exit code answers. publish and verify
//     without their script are skipped.
//   - config.timeout, when set, is a duration (10m) bounding each script
//     run: past it the script's process group is killed and the operation
//     fails. Every operation refuses a value that is not a positive
//     duration.
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
	timeout, err := scriptTimeout(cfg["timeout"])
	if err != nil {
		return fail("%v", err)
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
		res, err := git(nil, nil, "rev-parse", "--verify", "--quiet", base+"^{commit}")
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
	return runBashScript(ctx, req, payload, key, cfg[key], timeout, dir, deps)
}

// scriptTimeout reads config.timeout, the bound on each script run: empty
// means none.
func scriptTimeout(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("config.timeout %q must be a positive duration such as 10m, 90s or 1h30m", raw)
	}
	return d, nil
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

// bashGitFunc runs one git command: stdin (nil for none) on its input,
// its output on stdout when set.
type bashGitFunc func(stdin []byte, stdout io.Writer, args ...string) (proc.Result, error)

// bashGit runs git in dir, with pathspecs taken literally.
func bashGit(ctx context.Context, dir string, deps BashDeps) bashGitFunc {
	runner := deps.Proc
	if runner == nil {
		runner = proc.ExecRunner{}
	}
	env := deps.Env
	if env == nil {
		env = os.Environ()
	}
	return func(stdin []byte, stdout io.Writer, args ...string) (proc.Result, error) {
		return runner.Run(ctx, proc.Command{Name: "git", Args: append([]string{"-C", dir, "--literal-pathspecs"}, args...), Env: env, Stdin: stdin, Stdout: stdout})
	}
}

// checkCommitScript checks that the configured script clean is a regular
// file (not a symlink) in commit, named by label in the refusal.
func checkCommitScript(git bashGitFunc, commit, label, key, raw, clean string) error {
	res, err := git(nil, nil, "ls-tree", "-z", commit, "--", clean)
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
// commit's tree, killing its process group once timeout (when not 0) has
// passed.
func runBashScript(ctx context.Context, req extension.Request, p bashPayload, key, raw string, timeout time.Duration, dir string, deps BashDeps) extension.Response {
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
	res, err := git(nil, nil, "rev-parse", "--verify", "--quiet", "refs/tags/"+tag+"^{commit}")
	commit := strings.TrimSpace(string(res.Stdout))
	if tag == "" || err != nil || commit == "" {
		return fail("the release tag %s is not in %s: release-script runs in nerv deploy's clone of the release", orDash(tag), dir)
	}
	if err := checkCommitScript(git, commit, "the release commit "+tag, key, raw, clean); err != nil {
		return fail("%v", err)
	}

	tmp, err := os.MkdirTemp(deps.TempDir, "nerv-release-script-")
	if err != nil {
		return fail("%v", err)
	}
	defer os.RemoveAll(tmp)
	tree := filepath.Join(tmp, "tree")
	if err := extractCommit(git, commit, tmp, tree); err != nil {
		return fail("extracting the release commit %s: %v", tag, err)
	}

	call := extension.Call{Argv: []string{"bash", clean}, Dir: tree, Env: withVariables(env, bashVariables(req.Operation, p, commit)), Stderr: deps.Output}
	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	resp, _, err := extension.Exec(runCtx, runner, "script "+clean, call, req)
	switch {
	case err != nil && ctx.Err() == nil && errors.Is(runCtx.Err(), context.DeadlineExceeded):
		return fail("%s timed out after %s (config.timeout): its process group was killed", clean, timeout)
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

// extractCommit writes commit's tree into tree, read from the object
// database (git ls-tree and git cat-file) rather than git archive, so
// export-ignore and export-subst attributes do not change what the script
// finds. Executable modes are kept, a submodule is an empty folder, a
// symlink is made after every file (so no file is written through one),
// and an entry leaving tree is refused.
func extractCommit(git bashGitFunc, commit, tmp, tree string) error {
	res, err := git(nil, nil, "ls-tree", "-r", "-z", "--full-tree", commit)
	if err != nil {
		return fmt.Errorf("git ls-tree: %v: %s", err, extension.LastLine(res.Stderr))
	}
	type entry struct{ mode, kind, object, name string }
	var entries []entry
	var batch bytes.Buffer
	for _, raw := range bytes.Split(res.Stdout, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		meta, name, found := strings.Cut(string(raw), "\t")
		fields := strings.Fields(meta)
		if !found || len(fields) != 3 {
			return fmt.Errorf("git ls-tree: unexpected entry %q", raw)
		}
		if clean := path.Clean(name); clean != name || !filepath.IsLocal(filepath.FromSlash(name)) {
			return fmt.Errorf("tree entry %q leaves the tree", name)
		}
		e := entry{fields[0], fields[1], fields[2], name}
		entries = append(entries, e)
		if e.kind == "blob" {
			batch.WriteString(e.object + "\n")
		}
	}
	objects := filepath.Join(tmp, "objects")
	f, err := os.Create(objects)
	if err != nil {
		return err
	}
	res, err = git(batch.Bytes(), f, "cat-file", "--batch")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("git cat-file: %v: %s", err, extension.LastLine(res.Stderr))
	}
	f, err = os.Open(objects)
	if err != nil {
		return err
	}
	defer f.Close()
	objs := bufio.NewReader(f)
	if err := os.Mkdir(tree, 0o700); err != nil {
		return err
	}
	type link struct{ name, target string }
	var links []link
	for _, e := range entries {
		dest := filepath.Join(tree, filepath.FromSlash(e.name))
		if e.kind != "blob" {
			// A submodule (a commit entry): an empty folder, as git
			// archive leaves it.
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			continue
		}
		body, err := nextObject(objs, e.object)
		if err != nil {
			return err
		}
		switch e.mode {
		case "120000":
			target, err := io.ReadAll(body)
			if err != nil {
				return err
			}
			links = append(links, link{dest, string(target)})
		case "100755":
			err = writeTreeFile(dest, body, 0o755)
		default:
			err = writeTreeFile(dest, body, 0o644)
		}
		if err != nil {
			return err
		}
		if _, err := io.Copy(io.Discard, body); err != nil {
			return err
		}
		if b, err := objs.ReadByte(); err != nil || b != '\n' {
			return fmt.Errorf("git cat-file: object %s is not terminated", e.object)
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

// nextObject reads the next git cat-file --batch header from r, which
// must be the blob object, and returns a reader of its content.
func nextObject(r *bufio.Reader, object string) (io.Reader, error) {
	header, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("git cat-file: reading object %s: %v", object, err)
	}
	fields := strings.Fields(header)
	if len(fields) != 3 || fields[0] != object || fields[1] != "blob" {
		return nil, fmt.Errorf("git cat-file: object %s: unexpected header %q", object, strings.TrimSpace(header))
	}
	size, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || size < 0 {
		return nil, fmt.Errorf("git cat-file: object %s: bad size %q", object, fields[2])
	}
	return io.LimitReader(r, size), nil
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

// bashVariables are the deploy variables a release-script script finds in its
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
