package shiprun

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/proc"
	"github.com/war-apps/nerv-by-gentle-ai/internal/shipconfig"
)

// fakeProc records the command it gets and answers with handle.
type fakeProc struct {
	got      proc.Command
	deadline time.Time
	hasDL    bool
	handle   func(ctx context.Context, c proc.Command) (proc.Result, error)
}

func (f *fakeProc) Run(ctx context.Context, c proc.Command) (proc.Result, error) {
	f.got = c
	f.deadline, f.hasDL = ctx.Deadline()
	if f.handle != nil {
		return f.handle(ctx, c)
	}
	return proc.Result{}, nil
}

// exitErr stands for *exec.ExitError: an error with an exit code.
type exitErr struct {
	code int
	msg  string
}

func (e exitErr) Error() string { return e.msg }
func (e exitErr) ExitCode() int { return e.code }

// repo returns a repository root holding the given files.
func repo(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// dot is a path relative to the working directory, written ./x/y.
func dot(parts ...string) string {
	return "." + string(filepath.Separator) + filepath.Join(parts...)
}

func lookPathIn(dir string) func(string) (string, error) {
	return func(name string) (string, error) { return filepath.Join(dir, name), nil }
}

func newRunner(root string, f *fakeProc) *Runner {
	return &Runner{
		RepoRoot: root,
		Proc:     f,
		LookPath: lookPathIn("/bin"),
		Environ:  func() []string { return []string{"PATH=/bin", "HOME=/home/dev"} },
	}
}

func TestRun_CommandPerShell(t *testing.T) {
	root := repo(t, "scripts/build.sh", "scripts/bump.ps1", "scripts/notes.mjs", "scripts/a.js", "scripts/b.cjs", "tools/x.txt", "sub/dir/.keep")
	tests := []struct {
		name     string
		cmd      shipconfig.Command
		wantName string
		wantArgs []string
		wantDir  string
	}{
		{"bash inline", shipconfig.Command{Run: "make build", Shell: "bash"}, "bash",
			[]string{"--noprofile", "--norc", "-e", "-o", "pipefail", "-c", "make build"}, "."},
		{"bash file", shipconfig.Command{Run: "scripts/build.sh", Shell: "bash"}, "bash",
			[]string{"--noprofile", "--norc", "./scripts/build.sh"}, "."},
		{"bash file with backslashes", shipconfig.Command{Run: `scripts\build.sh`, Shell: "bash"}, "bash",
			[]string{"--noprofile", "--norc", "./scripts/build.sh"}, "."},
		{"bash file with arguments is inline", shipconfig.Command{Run: "scripts/build.sh --fast", Shell: "bash"}, "bash",
			[]string{"--noprofile", "--norc", "-e", "-o", "pipefail", "-c", "scripts/build.sh --fast"}, "."},
		{"bash missing file is inline", shipconfig.Command{Run: "scripts/nope.sh", Shell: "bash"}, "bash",
			[]string{"--noprofile", "--norc", "-e", "-o", "pipefail", "-c", "scripts/nope.sh"}, "."},
		{"bash directory is inline", shipconfig.Command{Run: "scripts", Shell: "bash"}, "bash",
			[]string{"--noprofile", "--norc", "-e", "-o", "pipefail", "-c", "scripts"}, "."},
		{"bash escaping path is inline", shipconfig.Command{Run: "../outside.sh", Shell: "bash"}, "bash",
			[]string{"--noprofile", "--norc", "-e", "-o", "pipefail", "-c", "../outside.sh"}, "."},
		{"bash file from a workdir", shipconfig.Command{Run: "scripts/build.sh", Shell: "bash", Workdir: "sub/dir"}, "bash",
			[]string{"--noprofile", "--norc", "../../scripts/build.sh"}, "sub/dir"},
		{"pwsh inline", shipconfig.Command{Run: "Write-Output hi", Shell: "pwsh"}, "pwsh",
			[]string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", pwshPrelude + "Write-Output hi" + pwshEpilogue}, "."},
		{"pwsh file", shipconfig.Command{Run: "scripts/bump.ps1", Shell: "pwsh"}, "pwsh",
			[]string{"-NoLogo", "-NoProfile", "-NonInteractive", "-File", dot("scripts", "bump.ps1")}, "."},
		{"pwsh non-ps1 file is inline", shipconfig.Command{Run: "tools/x.txt", Shell: "pwsh"}, "pwsh",
			[]string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", pwshPrelude + "tools/x.txt" + pwshEpilogue}, "."},
		{"node inline", shipconfig.Command{Run: "console.log(1)", Shell: "node"}, "node",
			[]string{"-e", "console.log(1)"}, "."},
		{"node mjs file", shipconfig.Command{Run: "scripts/notes.mjs", Shell: "node"}, "node",
			[]string{dot("scripts", "notes.mjs")}, "."},
		{"node js file", shipconfig.Command{Run: " scripts/a.js ", Shell: "node"}, "node",
			[]string{dot("scripts", "a.js")}, "."},
		{"node cjs file", shipconfig.Command{Run: "scripts/b.cjs", Shell: "node"}, "node",
			[]string{dot("scripts", "b.cjs")}, "."},
		{"node non-js file is inline", shipconfig.Command{Run: "scripts/build.sh", Shell: "node"}, "node",
			[]string{"-e", "scripts/build.sh"}, "."},
		{"default shell is bash", shipconfig.Command{Run: "true"}, "bash",
			[]string{"--noprofile", "--norc", "-e", "-o", "pipefail", "-c", "true"}, "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeProc{}
			if err := newRunner(root, f).Run(context.Background(), Step{Name: "build", Command: tt.cmd}, Vars{Project: "api"}); err != nil {
				t.Fatal(err)
			}
			if f.got.Name != filepath.Join("/bin", tt.wantName) {
				t.Errorf("name = %q, want %q", f.got.Name, filepath.Join("/bin", tt.wantName))
			}
			if !reflect.DeepEqual(f.got.Args, tt.wantArgs) {
				t.Errorf("args = %q\nwant   %q", f.got.Args, tt.wantArgs)
			}
			if want := filepath.Join(root, filepath.FromSlash(tt.wantDir)); f.got.Dir != want {
				t.Errorf("dir = %q, want %q", f.got.Dir, want)
			}
			if f.got.Stdin != nil {
				t.Errorf("stdin = %q, want none", f.got.Stdin)
			}
			if f.hasDL {
				t.Errorf("a command without timeout got a deadline")
			}
		})
	}
}

func TestRun_MissingWorkdirIsAnError(t *testing.T) {
	root := repo(t, "file.txt")
	for _, wd := range []string{"missing", "file.txt"} {
		f := &fakeProc{}
		err := newRunner(root, f).Run(context.Background(), Step{Name: "test", Command: shipconfig.Command{Run: "true", Shell: "bash", Workdir: wd}}, Vars{Project: "api"})
		if err == nil || !strings.Contains(err.Error(), `working directory "`+wd+`"`) {
			t.Errorf("workdir %s: err = %v, want it named", wd, err)
		}
		if f.got.Name != "" {
			t.Errorf("workdir %s: the command ran", wd)
		}
	}
}

func TestRun_Environment(t *testing.T) {
	root := repo(t)
	f := &fakeProc{}
	r := newRunner(root, f)
	r.Environ = func() []string {
		return []string{"PATH=/bin", "HOOK_VAR=inherited", "NERV_RELEASE_STALE=old", "NERV_RELEASE_VERSION=0.0.1"}
	}
	vars := Vars{
		Project: "api", ProjectPath: "services/api", Version: "1.2.0-alpha.2", Channel: "alpha",
		Tag: "api-v1.2.0-alpha.2", GitHead: "abc123", LastVersion: "1.2.0-alpha.1",
		NotesFile: "/tmp/notes.md", Branch: "develop", DryRun: true,
	}
	cmd := shipconfig.Command{Run: "true", Shell: "bash", Env: map[string]string{"HOOK_VAR": "from-hook", "OTHER": "x"}}
	if err := r.Run(context.Background(), Step{Name: "before_test", Command: cmd}, vars); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PATH=/bin", "HOOK_VAR=inherited",
		"HOOK_VAR=from-hook", "OTHER=x",
		"NERV_RELEASE_PROJECT=api",
		"NERV_RELEASE_PROJECT_PATH=services/api",
		"NERV_RELEASE_REPO_ROOT=" + root,
		"NERV_RELEASE_STAGE=before_test",
		"NERV_RELEASE_VERSION=1.2.0-alpha.2",
		"NERV_RELEASE_CHANNEL=alpha",
		"NERV_RELEASE_TAG=api-v1.2.0-alpha.2",
		"NERV_RELEASE_GIT_HEAD=abc123",
		"NERV_RELEASE_LAST_VERSION=1.2.0-alpha.1",
		"NERV_RELEASE_NOTES_FILE=/tmp/notes.md",
		"NERV_RELEASE_BRANCH=develop",
		"NERV_RELEASE_DRY_RUN=true",
	}
	if !reflect.DeepEqual(f.got.Env, want) {
		t.Errorf("env =\n%q\nwant\n%q", f.got.Env, want)
	}

	// Every variable is set, empty when unknown; DRY_RUN is false.
	if err := r.Run(context.Background(), Step{Name: "test", Command: shipconfig.Command{Run: "true"}}, Vars{Project: "api"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range EnvVars {
		if !slices.ContainsFunc(f.got.Env, func(kv string) bool { return strings.HasPrefix(kv, name+"=") }) {
			t.Errorf("%s not set", name)
		}
	}
	if !slices.Contains(f.got.Env, "NERV_RELEASE_DRY_RUN=false") || !slices.Contains(f.got.Env, "NERV_RELEASE_PROJECT_PATH=.") {
		t.Errorf("env = %q, want DRY_RUN=false and PROJECT_PATH=.", f.got.Env)
	}
}

func TestRun_TimeoutSetsADeadline(t *testing.T) {
	f := &fakeProc{}
	start := time.Now()
	cmd := shipconfig.Command{Run: "true", Timeout: "10m"}
	if err := newRunner(repo(t), f).Run(context.Background(), Step{Name: "test", Command: cmd}, Vars{Project: "api"}); err != nil {
		t.Fatal(err)
	}
	if !f.hasDL || f.deadline.Sub(start) < 9*time.Minute || f.deadline.Sub(start) > 11*time.Minute {
		t.Errorf("deadline = %v (set %v), want about 10m from now", f.deadline, f.hasDL)
	}
}

func TestRun_StreamsPrefixedLinesToOneWriter(t *testing.T) {
	f := &fakeProc{handle: func(_ context.Context, c proc.Command) (proc.Result, error) {
		if c.Stdout == nil || c.Stdout != c.Stderr {
			t.Errorf("stdout and stderr writers differ (%v, %v): one pipe keeps their order", c.Stdout, c.Stderr)
		}
		_, _ = c.Stdout.Write([]byte("one\ntw"))
		_, _ = c.Stderr.Write([]byte("o\r\nthree"))
		return proc.Result{Stdout: []byte("one\ntwo\r\nthree")}, nil
	}}
	var out bytes.Buffer
	r := newRunner(repo(t), f)
	r.Output = &out
	if err := r.Run(context.Background(), Step{Name: "after_build", Command: shipconfig.Command{Run: "x"}}, Vars{Project: "web-app"}); err != nil {
		t.Fatal(err)
	}
	want := "[web-app:after_build] one\n[web-app:after_build] two\r\n[web-app:after_build] three\n"
	if out.String() != want {
		t.Errorf("output:\n%q\nwant\n%q", out.String(), want)
	}
}

func TestPrefixWriter_LongLineWithoutNewline(t *testing.T) {
	var out bytes.Buffer
	w := newPrefixWriter(&out, "[p:s] ", 8)
	_, _ = w.Write([]byte("0123456789abc\nz"))
	w.Flush()
	want := "[p:s] 01234567\n[p:s] 89abc\n[p:s] z\n"
	if out.String() != want {
		t.Errorf("output %q, want %q", out.String(), want)
	}
}

func TestRun_Failures(t *testing.T) {
	root := repo(t, "scripts/build.sh")
	tail := strings.Repeat("noise\n", 30) + "error: it broke\n"
	tests := []struct {
		name     string
		cmd      shipconfig.Command
		ctx      func() (context.Context, context.CancelFunc)
		handle   func(ctx context.Context, c proc.Command) (proc.Result, error)
		want     []string
		notWant  []string
		is       error
		exitCode int
	}{
		{
			name: "exit code",
			cmd:  shipconfig.Command{Run: "scripts/build.sh", Env: map[string]string{"SECRET_TOKEN": "s3cr3t"}},
			handle: func(context.Context, proc.Command) (proc.Result, error) {
				return proc.Result{Stdout: []byte(tail)}, exitErr{3, "exit status 3"}
			},
			want:     []string{`project "api"`, `step "test"`, "bash", "scripts/build.sh", "exit code 3", "error: it broke"},
			notWant:  []string{"s3cr3t", "SECRET_TOKEN"},
			exitCode: 3,
		},
		{
			name: "signal",
			cmd:  shipconfig.Command{Run: "make", Shell: "bash"},
			handle: func(context.Context, proc.Command) (proc.Result, error) {
				return proc.Result{}, exitErr{-1, "signal: killed"}
			},
			want:     []string{"signal: killed"},
			exitCode: -1,
		},
		{
			name: "timeout",
			cmd:  shipconfig.Command{Run: "sleep 60", Timeout: "50ms"},
			handle: func(ctx context.Context, _ proc.Command) (proc.Result, error) {
				<-ctx.Done()
				return proc.Result{Stdout: []byte("waiting\n")}, exitErr{-1, "signal: killed"}
			},
			want:     []string{"timed out after 50ms", "waiting"},
			is:       ErrTimeout,
			exitCode: -1,
		},
		{
			name: "interrupted",
			cmd:  shipconfig.Command{Run: "sleep 60", Timeout: "1h"},
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			handle: func(ctx context.Context, _ proc.Command) (proc.Result, error) {
				return proc.Result{}, ctx.Err()
			},
			want:     []string{"interrupted"},
			is:       context.Canceled,
			exitCode: -1,
		},
		{
			name: "start failure",
			cmd:  shipconfig.Command{Run: "x", Shell: "node"},
			handle: func(context.Context, proc.Command) (proc.Result, error) {
				return proc.Result{}, errors.New("fork/exec /bin/node: permission denied")
			},
			want:     []string{"node", "permission denied"},
			exitCode: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.Background(), context.CancelFunc(func() {})
			if tt.ctx != nil {
				ctx, cancel = tt.ctx()
			}
			defer cancel()
			err := newRunner(root, &fakeProc{handle: tt.handle}).Run(ctx, Step{Name: "test", Command: tt.cmd}, Vars{Project: "api"})
			var se *StepError
			if !errors.As(err, &se) {
				t.Fatalf("err = %v, want a *StepError", err)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error:\n%v\nwant it to contain %q", err, w)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(err.Error(), w) {
					t.Errorf("error:\n%v\nmust not contain %q", err, w)
				}
			}
			if strings.Count(err.Error(), "noise") > maxTailLines {
				t.Errorf("error carries more than %d output lines", maxTailLines)
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("errors.Is(%v, %v) = false", err, tt.is)
			}
			if se.ExitCode != tt.exitCode || se.Project != "api" || se.Step != "test" {
				t.Errorf("StepError = %+v", se)
			}
		})
	}
}

func TestRun_MissingInterpreter(t *testing.T) {
	tests := []struct {
		shell, goos string
		want        string
	}{
		{"bash", "linux", "bash not found on PATH"},
		{"bash", "windows", "Git for Windows"},
		{"pwsh", "linux", "PowerShell 7"},
		{"pwsh", "windows", "PowerShell 7"},
		{"node", "darwin", "Node.js"},
	}
	for _, tt := range tests {
		t.Run(tt.shell+"/"+tt.goos, func(t *testing.T) {
			f := &fakeProc{}
			r := newRunner(repo(t), f)
			r.goos = tt.goos
			r.LookPath = func(string) (string, error) { return "", errors.New("not found") }
			r.fileExists = func(string) bool { return false }
			r.getenv = func(string) string { return "" }
			err := r.Run(context.Background(), Step{Name: "test", Command: shipconfig.Command{Run: "x", Shell: tt.shell}}, Vars{Project: "api"})
			if !errors.Is(err, ErrInterpreterNotFound) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want ErrInterpreterNotFound mentioning %q", err, tt.want)
			}
			if f.got.Name != "" {
				t.Error("a command ran without its interpreter")
			}
		})
	}
}

// On Windows, bash is Git Bash: WSL's launcher in System32 (or its
// WindowsApps alias) is skipped, and Git Bash is found next to git.exe or
// under Program Files.
func TestWindowsBash(t *testing.T) {
	sys32 := filepath.Join("C:", "Windows", "System32", "bash.exe")
	apps := filepath.Join("C:", "Users", "dev", "AppData", "Local", "Microsoft", "WindowsApps", "bash.exe")
	gitRoot := filepath.Join("D:", "tools", "Git")
	gitBash := filepath.Join(gitRoot, "bin", "bash.exe")
	pf := filepath.Join("C:", "Program Files")
	pfBash := filepath.Join(pf, "Git", "bin", "bash.exe")
	tests := []struct {
		name   string
		path   map[string]string
		exists []string
		want   string
	}{
		{"git bash on PATH", map[string]string{"bash": gitBash}, nil, gitBash},
		{"wsl launcher skipped for git cmd dir", map[string]string{"bash": sys32, "git": filepath.Join(gitRoot, "cmd", "git.exe")}, []string{gitBash}, gitBash},
		{"windowsapps alias skipped for mingw git", map[string]string{"bash": apps, "git": filepath.Join(gitRoot, "mingw64", "bin", "git.exe")}, []string{gitBash}, gitBash},
		{"program files fallback", map[string]string{"bash": sys32}, []string{pfBash}, pfBash},
		{"only wsl", map[string]string{"bash": sys32}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Runner{
				goos: "windows",
				LookPath: func(name string) (string, error) {
					if p, ok := tt.path[name]; ok {
						return p, nil
					}
					return "", errors.New("not found")
				},
				fileExists: func(p string) bool { return slices.Contains(tt.exists, p) },
				getenv: func(k string) string {
					if k == "ProgramFiles" {
						return pf
					}
					return ""
				},
			}
			got, err := r.interpreter("bash")
			if tt.want == "" {
				if !errors.Is(err, ErrInterpreterNotFound) || !strings.Contains(err.Error(), "WSL") {
					t.Errorf("interpreter = %q, %v; want ErrInterpreterNotFound mentioning WSL", got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("interpreter = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
