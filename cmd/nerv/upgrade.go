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
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/upgrade"
	"github.com/war-apps/nerv-by-gentle-ai/internal/version"
)

const upgradeUsage = `Usage: nerv upgrade [--alpha | --rc]

Replace this nerv binary with the newest release, then run the new
binary's install (nerv install, or nerv install --no-configure without an
interactive terminal). It downloads the release archive for this platform
and checksums.txt from GitHub, verifies the archive's SHA-256, and swaps the
binary in place; a failed download or checksum leaves the current binary
untouched. Already on the newest release (or newer), it changes nothing.

Flags:
  --alpha    Also consider alpha prereleases (the newest of stable and alpha)
  --rc       Also consider release candidates (the newest of stable and rc)

Exit codes: 0 up to date, or the new binary's install exit code;
1 the upgrade failed or this platform has no prebuilt binary; 2 usage error.
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
	channel := upgrade.Stable
	switch {
	case *alpha:
		channel = upgrade.Alpha
	case *rc:
		channel = upgrade.RC
	}

	signalCtx := opts.SignalContext
	if signalCtx == nil {
		signalCtx = signalContext
	}
	ctx, stop, _ := signalCtx(context.Background())
	defer stop()

	bin, err := replaceBinary(ctx, channel, stdout, opts)
	if err != nil {
		fmt.Fprintf(stderr, "nerv upgrade: %v\n", err)
		return 1
	}
	if bin == "" {
		return 0
	}
	return runNewInstall(ctx, bin, stdout, stderr, opts)
}

// replaceBinary swaps the running binary for the newest release of channel
// and returns its path, or "" when it is already up to date.
func replaceBinary(ctx context.Context, channel upgrade.Channel, stdout io.Writer, opts options) (string, error) {
	goos, goarch := opts.GOOS, opts.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if err := upgrade.CheckPlatform(goos, goarch); err != nil {
		return "", err
	}

	client := upgrade.Client{HTTP: opts.HTTPClient, APIBase: opts.ReleaseAPIBase, DownloadBase: opts.ReleaseDownloadBase}
	if client.HTTP == nil {
		client.HTTP = &http.Client{Timeout: upgradeHTTPTimeout}
	}
	if client.APIBase == "" {
		client.APIBase = upgrade.DefaultAPIBase
	}
	if client.DownloadBase == "" {
		client.DownloadBase = upgrade.DefaultDownloadBase
	}

	current := version.Binary
	fmt.Fprintf(stdout, "Current version: %s\n", current)
	tag, err := client.Resolve(ctx, channel)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(stdout, "Newest release on the %s channel: %s\n", channel, tag)
	if !upgrade.IsNewer(tag, current) {
		fmt.Fprintf(stdout, "nerv %s is up to date (newest on the %s channel: %s).\n", current, channel, tag)
		return "", nil
	}

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

	dir, err := os.MkdirTemp("", "nerv-upgrade-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	fmt.Fprintf(stdout, "Downloading %s\n", upgrade.ArchiveName(tag, goos, goarch))
	archive, err := client.Download(ctx, tag, goos, goarch, dir)
	if err != nil {
		return "", err
	}
	fmt.Fprintln(stdout, "Checksum verified.")
	if err := upgrade.Replace(archive, exe); err != nil {
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
