package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
)

// fakeRelease is one GitHub release the upgrade world publishes.
type fakeRelease struct {
	Tag        string
	Prerelease bool
}

// installCall records one run of the new binary.
type installCall struct {
	Bin  string
	Args []string
}

// upgradeWorld is a GitHub releases API and download server, and a running
// nerv binary in a temp dir, for nerv upgrade.
type upgradeWorld struct {
	t        *testing.T
	releases []fakeRelease // newest first, as GitHub returns them
	// corrupt makes checksums.txt list a wrong sum for every archive.
	corrupt    bool
	srv        *httptest.Server
	exe        string
	downloads  []string
	installs   []installCall
	installRC  int
	opts       options
	isTerminal bool
	// script, when set, is the new nerv binary in every archive.
	script string
	// gentleLatest is gentle-ai's latest stable release tag ("" serves 404).
	gentleLatest string
	// gentleRequests counts requests to gentle-ai's releases API.
	gentleRequests int
	// gentleRC is the exit code of a gentle-ai run, by its arguments.
	gentleRC map[string]int
}

const oldBinary = "#!old nerv binary\n"

func newBinaryFor(tag string) string { return "#!new nerv binary " + tag + "\n" }

func newUpgradeWorld(t *testing.T, current string, releases ...fakeRelease) *upgradeWorld {
	t.Helper()
	w := &upgradeWorld{t: t, releases: releases}
	w.srv = httptest.NewServer(http.HandlerFunc(w.serve))
	t.Cleanup(w.srv.Close)

	dir := t.TempDir()
	w.exe = filepath.Join(dir, "nerv")
	if err := os.WriteFile(w.exe, []byte(oldBinary), 0o755); err != nil {
		t.Fatal(err)
	}

	previous := version.Binary
	version.Binary = current
	t.Cleanup(func() { version.Binary = previous })

	w.opts = testOptions(t.TempDir())
	w.opts.HTTPClient = w.srv.Client()
	w.opts.ReleaseAPIBase = w.srv.URL + "/api"
	w.opts.ReleaseDownloadBase = w.srv.URL + "/download"
	w.opts.GentleAIAPIBase = w.srv.URL + "/gentle"
	w.opts.Executable = func() (string, error) { return w.exe, nil }
	w.opts.GOOS, w.opts.GOARCH = "linux", "amd64"
	w.opts.IsTerminal = func() bool { return w.isTerminal }
	w.opts.RunBinary = func(_ context.Context, bin string, args []string, stdout, _ io.Writer) (int, error) {
		w.installs = append(w.installs, installCall{Bin: bin, Args: args})
		if bin == "gentle-ai" {
			return w.gentleRC[strings.Join(args, " ")], nil
		}
		fmt.Fprintln(stdout, "install output")
		return w.installRC, nil
	}
	return w
}

func archiveName(tag, goos, goarch string) string {
	return "nerv_" + strings.TrimPrefix(tag, "v") + "_" + goos + "_" + goarch + ".tar.gz"
}

// archive builds the release archive of tag: a tar.gz holding nerv.
func (w *upgradeWorld) archive(tag string) []byte {
	t := w.t
	t.Helper()
	body := newBinaryFor(tag)
	if w.script != "" {
		body = w.script
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct{ name, body string }{{"README.md", "readme\n"}, {"nerv", body}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (w *upgradeWorld) serve(rw http.ResponseWriter, r *http.Request) {
	type apiRelease struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}
	switch {
	case r.URL.Path == "/gentle/releases/latest":
		w.gentleRequests++
		if w.gentleLatest == "" {
			http.NotFound(rw, r)
			return
		}
		_ = json.NewEncoder(rw).Encode(apiRelease{TagName: w.gentleLatest})
	case r.URL.Path == "/api/releases/latest":
		for _, rel := range w.releases {
			if !rel.Prerelease {
				_ = json.NewEncoder(rw).Encode(apiRelease{TagName: rel.Tag})
				return
			}
		}
		http.NotFound(rw, r)
	case r.URL.Path == "/api/releases" && r.URL.Query().Get("per_page") == "50":
		list := []apiRelease{}
		for _, rel := range w.releases {
			list = append(list, apiRelease{TagName: rel.Tag, Prerelease: rel.Prerelease})
		}
		_ = json.NewEncoder(rw).Encode(list)
	case strings.HasPrefix(r.URL.Path, "/download/"):
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/download/"), "/")
		if len(parts) != 2 {
			http.NotFound(rw, r)
			return
		}
		tag, file := parts[0], parts[1]
		w.downloads = append(w.downloads, tag+"/"+file)
		switch file {
		case archiveName(tag, "linux", "amd64"):
			_, _ = rw.Write(w.archive(tag))
		case "checksums.txt":
			sum := sha256.Sum256(w.archive(tag))
			hexSum := hex.EncodeToString(sum[:])
			if w.corrupt {
				hexSum = strings.Repeat("0", 64)
			}
			fmt.Fprintf(rw, "%s  %s\n%s  %s\n", strings.Repeat("a", 64), archiveName(tag, "darwin", "arm64"), hexSum, archiveName(tag, "linux", "amd64"))
		default:
			http.NotFound(rw, r)
		}
	default:
		http.NotFound(rw, r)
	}
}

func (w *upgradeWorld) upgrade(args ...string) (int, string, string) {
	w.t.Helper()
	var out, errOut bytes.Buffer
	code := run(append([]string{"upgrade"}, args...), &out, &errOut, w.opts)
	return code, out.String(), errOut.String()
}

func (w *upgradeWorld) binary() string {
	w.t.Helper()
	data, err := os.ReadFile(w.exe)
	if err != nil {
		w.t.Fatal(err)
	}
	return string(data)
}

func (w *upgradeWorld) assertUntouched() {
	w.t.Helper()
	if got := w.binary(); got != oldBinary {
		w.t.Errorf("binary = %q, want it untouched (%q)", got, oldBinary)
	}
	entries, err := os.ReadDir(filepath.Dir(w.exe))
	if err != nil {
		w.t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		w.t.Errorf("executable dir holds %v, want only nerv", names)
	}
	if len(w.installs) != 0 {
		w.t.Errorf("install ran: %+v", w.installs)
	}
}

var defaultReleases = []fakeRelease{
	{Tag: "v5.2.0-rc.1", Prerelease: true},
	{Tag: "v5.2.0-alpha.3", Prerelease: true},
	{Tag: "v5.1.0"},
	{Tag: "v5.1.0-rc.2", Prerelease: true},
	{Tag: "v5.0.0"},
}

// The default channel installs the newest stable release, replaces the
// running binary with mode 0755, and runs the new binary's install
// (--no-configure without a terminal), propagating its exit code.
func TestUpgrade_StableReplacesBinaryAndRunsInstall(t *testing.T) {
	w := newUpgradeWorld(t, "v5.0.0", defaultReleases...)
	w.installRC = 3
	code, out, errOut := w.upgrade()
	if code != 3 {
		t.Fatalf("exit %d, want install's 3; stderr %q\n%s", code, errOut, out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty", errOut)
	}
	for _, want := range []string{
		"Current version: v5.0.0\n",
		"Newest release on the stable channel: v5.1.0\n",
		"Downloading nerv_5.1.0_linux_amd64.tar.gz\n",
		"Checksum verified.\n",
		"Replaced " + w.exe + "\n",
		"No interactive terminal available; running: " + w.exe + " install --no-configure\n",
		"install output\n",
		"Run '" + w.exe + " configure' later to finish setup.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if got := w.binary(); got != newBinaryFor("v5.1.0") {
		t.Errorf("binary = %q, want the v5.1.0 one", got)
	}
	info, err := os.Stat(w.exe)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(w.exe))
	if len(entries) != 1 {
		t.Errorf("executable dir holds %d entries, want only nerv", len(entries))
	}
	want := []installCall{{Bin: w.exe, Args: []string{"install", "--no-configure"}}}
	if fmt.Sprint(w.installs) != fmt.Sprint(want) {
		t.Errorf("installs = %+v, want %+v", w.installs, want)
	}
	if fmt.Sprint(w.downloads) != fmt.Sprint([]string{"v5.1.0/nerv_5.1.0_linux_amd64.tar.gz", "v5.1.0/checksums.txt"}) {
		t.Errorf("downloads = %v", w.downloads)
	}
}

// With a terminal, install runs plainly (its wizard can prompt); a
// symlinked executable is resolved: its target is replaced, the link kept.
func TestUpgrade_TerminalRunsInstallAndResolvesSymlink(t *testing.T) {
	w := newUpgradeWorld(t, "dev", defaultReleases...)
	w.isTerminal = true
	link := filepath.Join(t.TempDir(), "nerv")
	if err := os.Symlink(w.exe, link); err != nil {
		t.Fatal(err)
	}
	w.opts.Executable = func() (string, error) { return link, nil }
	code, out, errOut := w.upgrade()
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
	}
	if !strings.Contains(out, "Current version: dev\n") || !strings.Contains(out, "Running: "+w.exe+" install\n") {
		t.Errorf("stdout:\n%s", out)
	}
	if got := w.binary(); got != newBinaryFor("v5.1.0") {
		t.Errorf("binary = %q, want the v5.1.0 one", got)
	}
	if target, err := os.Readlink(link); err != nil || target != w.exe {
		t.Errorf("link = %q, %v; want it kept, pointing at %s", target, err, w.exe)
	}
	want := []installCall{{Bin: w.exe, Args: []string{"install"}}}
	if fmt.Sprint(w.installs) != fmt.Sprint(want) {
		t.Errorf("installs = %+v, want %+v", w.installs, want)
	}
}

// Without an injected runner, the new binary really runs, with its output
// streamed and its exit code propagated.
func TestUpgrade_RunsTheNewBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake new binary is a shell script")
	}
	w := newUpgradeWorld(t, "v5.0.0", defaultReleases...)
	w.script = "#!/bin/sh\necho \"new nerv $*\"\necho oops >&2\nexit 4\n"
	w.opts.RunBinary = nil
	code, out, errOut := w.upgrade()
	if code != 4 {
		t.Fatalf("exit %d, want 4; stderr %q\n%s", code, errOut, out)
	}
	if !strings.Contains(out, "\nnew nerv install --no-configure\n") || errOut != "oops\n" {
		t.Errorf("stdout:\n%s\nstderr %q", out, errOut)
	}
	if got := w.binary(); got != w.script {
		t.Errorf("binary = %q, want the script", got)
	}
}

// --alpha and --rc pick the newest release of their channel, or the
// newest stable release when it is newer than every prerelease of it.
func TestUpgrade_PrereleaseChannels(t *testing.T) {
	newerStable := []fakeRelease{
		{Tag: "v5.3.0"},
		{Tag: "v5.2.0-rc.1", Prerelease: true},
		{Tag: "v5.2.0-alpha.3", Prerelease: true},
	}
	tests := []struct {
		name     string
		flag     string
		releases []fakeRelease
		want     string
		label    string
	}{
		{"alpha", "--alpha", defaultReleases, "v5.2.0-alpha.3", "alpha"},
		{"rc", "--rc", defaultReleases, "v5.2.0-rc.1", "rc"},
		{"alpha behind stable", "--alpha", newerStable, "v5.3.0", "alpha"},
		{"rc behind stable", "--rc", newerStable, "v5.3.0", "rc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newUpgradeWorld(t, "v5.0.0", tt.releases...)
			code, out, errOut := w.upgrade(tt.flag)
			if code != 0 || errOut != "" {
				t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
			}
			if !strings.Contains(out, "Newest release on the "+tt.label+" channel: "+tt.want+"\n") {
				t.Errorf("stdout lacks the %s release line:\n%s", tt.want, out)
			}
			if got := w.binary(); got != newBinaryFor(tt.want) {
				t.Errorf("binary = %q, want the %s one", got, tt.want)
			}
			if len(w.installs) != 1 {
				t.Errorf("installs = %+v, want one", w.installs)
			}
		})
	}
}

// Already on the newest release of the channel, or on a newer one: it
// says so, changes nothing and does not run install.
func TestUpgrade_UpToDate(t *testing.T) {
	tests := []struct{ name, current, flag, newest string }{
		{"same stable", "v5.1.0", "", "v5.1.0"},
		{"same without v", "5.1.0", "", "v5.1.0"},
		{"newer than stable", "v5.2.0-rc.1", "", "v5.1.0"},
		{"same rc", "v5.2.0-rc.1", "--rc", "v5.2.0-rc.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newUpgradeWorld(t, tt.current, defaultReleases...)
			args := []string{}
			if tt.flag != "" {
				args = append(args, tt.flag)
			}
			code, out, errOut := w.upgrade(args...)
			if code != 0 || errOut != "" {
				t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
			}
			if !strings.Contains(out, "nerv "+tt.current+" is up to date (newest") || !strings.Contains(out, tt.newest+").\n") {
				t.Errorf("stdout lacks the up-to-date line:\n%s", out)
			}
			w.assertUntouched()
			if len(w.downloads) != 0 {
				t.Errorf("downloads = %v, want none", w.downloads)
			}
		})
	}
}

// A checksum mismatch or a failed download leaves the binary untouched,
// runs nothing, and exits 1 with the error.
func TestUpgrade_FailuresLeaveBinaryUntouched(t *testing.T) {
	t.Run("checksum mismatch", func(t *testing.T) {
		w := newUpgradeWorld(t, "v5.0.0", defaultReleases...)
		w.corrupt = true
		code, out, errOut := w.upgrade()
		if code != 1 {
			t.Fatalf("exit %d, want 1\n%s", code, out)
		}
		if errOut != "nerv upgrade: Checksum verification failed for nerv_5.1.0_linux_amd64.tar.gz.\n" {
			t.Errorf("stderr = %q", errOut)
		}
		if strings.Contains(out, "Replaced") {
			t.Errorf("stdout claims a replacement:\n%s", out)
		}
		w.assertUntouched()
	})
	t.Run("missing archive", func(t *testing.T) {
		w := newUpgradeWorld(t, "v5.0.0", defaultReleases...)
		w.opts.GOARCH = "arm64"
		code, out, errOut := w.upgrade()
		if code != 1 {
			t.Fatalf("exit %d, want 1\n%s", code, out)
		}
		wantURL := w.srv.URL + "/download/v5.1.0/nerv_5.1.0_linux_arm64.tar.gz"
		if errOut != "nerv upgrade: downloading "+wantURL+": 404 Not Found\n" {
			t.Errorf("stderr = %q", errOut)
		}
		w.assertUntouched()
	})
	t.Run("no prerelease of the channel", func(t *testing.T) {
		w := newUpgradeWorld(t, "v5.0.0", fakeRelease{Tag: "v5.2.0-rc.1", Prerelease: true})
		code, out, errOut := w.upgrade("--alpha")
		if code != 1 {
			t.Fatalf("exit %d, want 1\n%s", code, out)
		}
		if errOut != "nerv upgrade: No alpha release published yet.\n" {
			t.Errorf("stderr = %q", errOut)
		}
		w.assertUntouched()
	})
	t.Run("latest stable tag is not a version", func(t *testing.T) {
		w := newUpgradeWorld(t, "v5.0.0", fakeRelease{Tag: "v9.9.9/../../x"})
		code, out, errOut := w.upgrade()
		if code != 1 {
			t.Fatalf("exit %d, want 1\n%s", code, out)
		}
		if !strings.HasPrefix(errOut, `nerv upgrade: The latest stable release tag "v9.9.9/../../x" is not a version`) {
			t.Errorf("stderr = %q", errOut)
		}
		if len(w.downloads) != 0 {
			t.Errorf("downloads = %v, want none", w.downloads)
		}
		w.assertUntouched()
	})
}

// A platform without a prebuilt binary is refused before any request.
func TestUpgrade_UnsupportedPlatformIsRefused(t *testing.T) {
	tests := []struct{ goos, goarch, want string }{
		{"windows", "amd64", "nerv upgrade: Unsupported OS: windows. Only Linux and macOS have prebuilt nerv binaries.\nInstall with: go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest\n"},
		{"linux", "386", "nerv upgrade: Unsupported architecture: 386. Only amd64 and arm64 have prebuilt nerv binaries.\nInstall with: go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest\n"},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			w := newUpgradeWorld(t, "v5.0.0", defaultReleases...)
			w.opts.GOOS, w.opts.GOARCH = tt.goos, tt.goarch
			code, out, errOut := w.upgrade()
			if code != 1 {
				t.Fatalf("exit %d, want 1\n%s", code, out)
			}
			if errOut != tt.want {
				t.Errorf("stderr = %q, want %q", errOut, tt.want)
			}
			if out != "" {
				t.Errorf("stdout = %q, want empty", out)
			}
			w.assertUntouched()
			if len(w.downloads) != 0 {
				t.Errorf("downloads = %v, want none", w.downloads)
			}
		})
	}
}

// --alpha with --rc, an unknown flag or an argument is a usage error;
// --help prints the usage.
func TestUpgrade_Usage(t *testing.T) {
	for _, args := range [][]string{{"--alpha", "--rc"}, {"--beta"}, {"now"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			w := newUpgradeWorld(t, "v5.0.0", defaultReleases...)
			code, out, errOut := w.upgrade(args...)
			if code != 2 {
				t.Fatalf("exit %d, want 2", code)
			}
			if out != "" || !strings.Contains(errOut, "Usage: nerv upgrade [--alpha | --rc]") {
				t.Errorf("stdout %q, stderr %q", out, errOut)
			}
			if args[0] == "--alpha" && !strings.HasPrefix(errOut, "nerv upgrade: --alpha and --rc cannot be used together\n") {
				t.Errorf("stderr = %q", errOut)
			}
			w.assertUntouched()
			if len(w.downloads) != 0 {
				t.Errorf("downloads = %v, want none", w.downloads)
			}
		})
	}
	w := newUpgradeWorld(t, "v5.0.0", defaultReleases...)
	code, out, errOut := w.upgrade("--help")
	if code != 0 || errOut != "" || !strings.HasPrefix(out, "Usage: nerv upgrade [--alpha | --rc]") {
		t.Errorf("--help: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	w.assertUntouched()
}
