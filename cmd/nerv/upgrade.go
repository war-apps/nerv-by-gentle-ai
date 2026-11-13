package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/gentleai"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
	"github.com/war-apps/nerv-kit-by-gentle-ai/selfupgrade"
)

const (
	// defaultAPIBase is the GitHub API base of nerv's own repository.
	defaultAPIBase = "https://api.github.com/repos/war-apps/nerv-by-gentle-ai"
	// gentleAIAPIBase is the GitHub API base of gentle-ai's repository.
	gentleAIAPIBase = "https://api.github.com/repos/Gentleman-Programming/gentle-ai"
	// defaultDownloadBase is where nerv's release assets are downloaded.
	defaultDownloadBase = "https://github.com/war-apps/nerv-by-gentle-ai/releases/download"

	goInstallHint = "Install with: go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest"
)

const upgradeUsage = `Usage: nerv upgrade [--alpha | --rc]

Replace this nerv binary with the newest release, then run the new
binary's install (nerv install, or nerv install --no-configure without an
interactive terminal). It downloads the release archive for this platform
and checksums.txt from GitHub, verifies the archive's SHA-256, and swaps the
binary in place; a failed download or checksum leaves the current binary
untouched. Already on the newest release (or newer), it changes nothing.

It also upgrades gentle-ai (gentle-ai upgrade gentle-ai, then gentle-ai
sync) when gentle-ai's newest release is newer and still 4.x, the major
version NERV supports; a new major version is reported, never installed.

Install still runs, with the running executable, even when nerv itself
was up to date (gentle-ai sync rewrote NERV's managed files) or could not
be replaced at all (an unsupported platform, a failed download or
checksum) — so gentle-ai keeps getting its chance and NERV stays laid
over it either way. That install is told not to upgrade gentle-ai again
(it already ran once, right here).

Flags:
  --alpha    Also consider alpha prereleases (the newest of stable and alpha)
  --rc       Also consider release candidates (the newest of stable and rc)

Exit codes: 0 up to date, or the new binary's install exit code;
1 the upgrade failed (nerv's or gentle-ai's) or this platform has no
prebuilt binary; 2 usage error.
`

// upgradeHTTPTimeout bounds every request nerv upgrade makes when
// options.HTTPClient is nil.
const upgradeHTTPTimeout = 5 * time.Minute

func runUpgrade(args []string, stdout, stderr io.Writer, opts options) int {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	alpha := fs.Bool("alpha", false, "")
	rc := fs.Bool("rc", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, upgradeUsage)
			return 0
		}
		fmt.Fprintf(stderr, "nerv upgrade: %v\n\n%s", err, upgradeUsage)
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "nerv upgrade: unexpected argument %q\n\n%s", fs.Arg(0), upgradeUsage)
		return 2
	}
	if *alpha && *rc {
		fmt.Fprintf(stderr, "nerv upgrade: --alpha and --rc cannot be used together\n\n%s", upgradeUsage)
		return 2
	}
	channel := selfupgrade.Stable
	switch {
	case *alpha:
		channel = selfupgrade.Alpha
	case *rc:
		channel = selfupgrade.RC
	}

	if exe, err := runningExecutable(opts); err == nil {
		// Best-effort: a leftover <exe>.old from an earlier windows
		// replacement (selfupgrade.Replace) that could not be
		// removed while it was still that run's image. Harmless on
		// every other goos, where Replace never creates one.
		selfupgrade.CleanupStaleReplacement(exe)
	}

	signalCtx := opts.SignalContext
	if signalCtx == nil {
		signalCtx = signalContext
	}
	ctx, stop, _ := signalCtx(context.Background())
	defer stop()

	bin, replaceErr := replaceBinary(ctx, channel, stdout, opts)
	if replaceErr != nil {
		fmt.Fprintf(stderr, "nerv upgrade: %v\n", replaceErr)
	}
	gentleUpgraded, gentleErr := upgradeGentleAI(ctx, stdout, stderr, opts)
	if gentleErr != nil {
		fmt.Fprintf(stderr, "nerv upgrade: gentle-ai: %v\n", gentleErr)
	}
	// Install still needs to run when: nerv itself could not be replaced
	// (S5 — gentle-ai already got its chance above, and NERV still needs
	// laying over with the running executable); gentle-ai sync rewrote
	// NERV's managed files even though nerv was up to date; or nerv was
	// replaced. Up to date with nothing upgraded runs nothing, as before.
	runInstall := replaceErr != nil || bin != "" || gentleUpgraded
	if bin == "" && runInstall {
		var err error
		if bin, err = runningExecutable(opts); err != nil {
			fmt.Fprintf(stderr, "nerv upgrade: %v\n", err)
			return 1
		}
	}
	code := 0
	if runInstall {
		code = runNewInstall(ctx, bin, stdout, stderr, opts)
	}
	if code == 0 && (replaceErr != nil || gentleErr != nil) {
		return 1
	}
	return code
}

// upgradeGentleAI upgrades gentle-ai with "gentle-ai upgrade gentle-ai",
// then runs "gentle-ai sync", when its newest release is newer than the
// installed one and of the major version NERV supports; it reports whether
// it did. gentle-ai upgrade installs the newest release, whatever its
// major version, so a newer major is never upgraded to.
func upgradeGentleAI(ctx context.Context, stdout, stderr io.Writer, opts options) (bool, error) {
	pre := gentleai.CheckPreflight(ctx, opts.Runner)
	switch {
	case !pre.Found:
		fmt.Fprintf(stdout, "gentle-ai is not on PATH; not upgrading it. NERV needs gentle-ai %d.x: %s\n", gentleai.SupportedMajor, gentleAIURL)
		return false, nil
	case pre.Version == "":
		fmt.Fprintln(stdout, "gentle-ai --version printed no version; not upgrading it.")
		return false, nil
	case !pre.OK:
		fmt.Fprintf(stdout, "gentle-ai %s is not %d.x, the major version NERV supports; not upgrading it. Install gentle-ai %d.x: %s\n", pre.Version, gentleai.SupportedMajor, gentleai.SupportedMajor, gentleAIURL)
		return false, nil
	}

	client := newUpgradeClient(opts)
	client.APIBase = opts.GentleAIAPIBase
	if client.APIBase == "" {
		client.APIBase = gentleAIAPIBase
	}
	latest, err := client.Resolve(ctx, selfupgrade.Stable)
	if err != nil {
		return false, err
	}
	fmt.Fprintf(stdout, "gentle-ai: installed %s, newest release %s\n", pre.Version, latest)
	if gentleai.Major(latest) != gentleai.SupportedMajor {
		fmt.Fprintf(stdout, "gentle-ai %s is a new major version: NERV %s supports gentle-ai %d.x, so gentle-ai stays on %s.\n", latest, version.Binary, gentleai.SupportedMajor, pre.Version)
		return false, nil
	}
	if !selfupgrade.IsNewer(latest, pre.Version) {
		fmt.Fprintf(stdout, "gentle-ai %s is up to date.\n", pre.Version)
		return false, nil
	}

	runBinary := opts.RunBinary
	if runBinary == nil {
		runBinary = execBinary
	}
	fmt.Fprintf(stdout, "Upgrading gentle-ai to %s: gentle-ai upgrade gentle-ai\n", latest)
	for i, args := range [][]string{{"upgrade", "gentle-ai"}, {"sync"}} {
		if i > 0 {
			fmt.Fprintf(stdout, "Running: gentle-ai %s\n", strings.Join(args, " "))
		}
		code, err := runBinary(ctx, "gentle-ai", args, stdout, stderr)
		if err != nil {
			return i > 0, fmt.Errorf("running gentle-ai %s: %w", strings.Join(args, " "), err)
		}
		if code != 0 {
			return i > 0, fmt.Errorf("gentle-ai %s exited %d", strings.Join(args, " "), code)
		}
	}
	return true, nil
}

// gentleAIURL is where gentle-ai is installed from.
const gentleAIURL = "https://github.com/Gentleman-Programming/gentle-ai"

// newUpgradeClient is the releases client of nerv's repository, from opts.
func newUpgradeClient(opts options) selfupgrade.Client {
	client := selfupgrade.Client{HTTP: opts.HTTPClient, APIBase: opts.ReleaseAPIBase, DownloadBase: opts.ReleaseDownloadBase, BinaryName: "nerv"}
	if client.HTTP == nil {
		client.HTTP = &http.Client{Timeout: upgradeHTTPTimeout}
	}
	if client.APIBase == "" {
		client.APIBase = defaultAPIBase
	}
	if client.DownloadBase == "" {
		client.DownloadBase = defaultDownloadBase
	}
	return client
}

// runningExecutable is the path of the running nerv binary, symlinks
// resolved.
func runningExecutable(opts options) (string, error) {
	executable := opts.Executable
	if executable == nil {
		executable = os.Executable
	}
	exe, err := executable()
	if err != nil {
		return "", fmt.Errorf("locating the running nerv binary: %w", err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", fmt.Errorf("locating the running nerv binary: %w", err)
	}
	return exe, nil
}

// replaceBinary swaps the running binary for the newest release of channel
// and returns its path, or "" when it is already up to date.
func replaceBinary(ctx context.Context, channel selfupgrade.Channel, stdout io.Writer, opts options) (string, error) {
	goos, goarch := opts.GOOS, opts.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if err := selfupgrade.CheckPlatform(goos, goarch, "nerv", goInstallHint); err != nil {
		return "", err
	}

	client := newUpgradeClient(opts)

	current := version.Binary
	fmt.Fprintf(stdout, "Current version: %s\n", current)
	tag, err := client.Resolve(ctx, channel)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(stdout, "Newest release on the %s channel: %s\n", channel, tag)
	if !selfupgrade.IsNewer(tag, current) {
		fmt.Fprintf(stdout, "nerv %s is up to date (newest on the %s channel: %s).\n", current, channel, tag)
		return "", nil
	}

	exe, err := runningExecutable(opts)
	if err != nil {
		return "", err
	}

	dir, err := os.MkdirTemp("", "nerv-upgrade-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	fmt.Fprintf(stdout, "Downloading %s\n", selfupgrade.ArchiveName("nerv", tag, goos, goarch))
	archive, err := client.Download(ctx, tag, goos, goarch, dir)
	if err != nil {
		return "", err
	}
	fmt.Fprintln(stdout, "Checksum verified.")
	if err := selfupgrade.Replace(archive, exe, goos, "nerv"); err != nil {
		return "", err
	}
	fmt.Fprintf(stdout, "Replaced %s\n", exe)
	return exe, nil
}

// runNewInstall runs the new binary's install as scripts/install.sh does:
// plainly with a terminal (its wizard can prompt), --no-configure
// otherwise. It returns install's exit code.
func runNewInstall(ctx context.Context, bin string, stdout, stderr io.Writer, opts options) int {
	args := []string{"install"}
	terminal := opts.IsTerminal != nil && opts.IsTerminal()
	if terminal {
		fmt.Fprintf(stdout, "Running: %s install\n", bin)
	} else {
		args = append(args, "--no-configure")
		fmt.Fprintf(stdout, "No interactive terminal available; running: %s install --no-configure\n", bin)
	}
	// upgradeGentleAI already ran gentle-ai's upgrade once for this nerv
	// upgrade (whether or not it actually upgraded anything): the install
	// this spawns must not run it again.
	args = append(args, "--skip-gentle-ai-upgrade")
	runBinary := opts.RunBinary
	if runBinary == nil {
		runBinary = execBinary
	}
	code, err := runBinary(ctx, bin, args, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "nerv upgrade: running %s install: %v\n", bin, err)
		return 1
	}
	if !terminal {
		fmt.Fprintf(stdout, "Run '%s configure' later to finish setup.\n", bin)
	}
	return code
}

// execBinary runs bin in the foreground with this process's standard input
// (the install wizard prompts on it), streaming its output, and returns its
// exit code. Unlike proc.ExecRunner it keeps the terminal: the child stays
// in nerv's process group, with the real stdin.
func execBinary(ctx context.Context, bin string, args []string, stdout, stderr io.Writer) (int, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code > 0 {
			return code, nil
		}
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}
