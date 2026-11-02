// Package upgrade replaces the running nerv binary with a newer GitHub
// release: it resolves the newest release of a channel, downloads the
// platform's archive and checksums.txt, verifies the archive's SHA-256,
// extracts the nerv binary and installs it over the running one — renamed
// directly over it on Linux and macOS, or renamed aside first on Windows,
// which refuses that while the binary is running (see Replace). It mirrors
// scripts/install.sh: the same release lookup, asset names, checksum check
// and supported platforms (Windows included).
package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/refusal"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
)

const (
	// DefaultAPIBase is the GitHub API base of nerv's repository.
	DefaultAPIBase = "https://api.github.com/repos/war-apps/nerv-by-gentle-ai"
	// GentleAIAPIBase is the GitHub API base of gentle-ai's repository.
	GentleAIAPIBase = "https://api.github.com/repos/Gentleman-Programming/gentle-ai"
	// DefaultDownloadBase is where nerv's release assets are downloaded.
	DefaultDownloadBase = "https://github.com/war-apps/nerv-by-gentle-ai/releases/download"

	goInstallHint = "Install with: go install github.com/war-apps/nerv-by-gentle-ai/cmd/nerv@latest"
	// maxBinarySize bounds the nerv binary extracted from an archive.
	maxBinarySize = 512 << 20
)

// Channel is a release channel: "stable", "alpha" or "rc".
type Channel string

const (
	Stable Channel = "stable"
	Alpha  Channel = "alpha"
	RC     Channel = "rc"
)

// CheckPlatform refuses an OS or architecture goreleaser builds no nerv
// binary for (only linux|darwin|windows and amd64|arm64).
func CheckPlatform(goos, goarch string) error {
	switch goos {
	case "linux", "darwin", "windows":
	default:
		return &refusal.Error{Err: fmt.Errorf("Unsupported OS: %s. Only Linux, macOS and Windows have prebuilt nerv binaries.\n%s", goos, goInstallHint)}
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return &refusal.Error{Err: fmt.Errorf("Unsupported architecture: %s. Only amd64 and arm64 have prebuilt nerv binaries.\n%s", goarch, goInstallHint)}
	}
	return nil
}

// IsNewer reports whether release is newer than current. A current version
// that is not a semantic version ("dev", an unreleased build) is older than
// any release.
func IsNewer(release, current string) bool {
	if !versioning.Valid(current) {
		return true
	}
	cmp, ok := versioning.Compare(release, current)
	return ok && cmp > 0
}

// Client talks to the releases API and the download server.
type Client struct {
	HTTP         *http.Client
	APIBase      string
	DownloadBase string
}

type release struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

// Resolve returns the tag to install for channel: the latest release for
// stable; for alpha or rc, the newest (by semver) of the stable releases and
// the channel's prereleases among the 50 most recent releases, so a newer
// stable release wins over an older prerelease.
func (c Client) Resolve(ctx context.Context, channel Channel) (string, error) {
	if channel == Stable {
		url := c.APIBase + "/releases/latest"
		var latest release
		if err := c.getJSON(ctx, url, &latest); err != nil {
			return "", err
		}
		if latest.TagName == "" {
			return "", fmt.Errorf("Could not resolve the latest stable release from %s.", url)
		}
		// The tag goes into download URLs and file names: only a version.
		if !versioning.Valid(latest.TagName) {
			return "", fmt.Errorf("The latest stable release tag %q is not a version (%s).", latest.TagName, url)
		}
		return latest.TagName, nil
	}

	var list []release
	if err := c.getJSON(ctx, c.APIBase+"/releases?per_page=50", &list); err != nil {
		return "", err
	}
	marker := "-" + string(channel) + "."
	best := ""
	for _, r := range list {
		if r.Draft {
			continue
		}
		if !versioning.Valid(r.TagName) {
			continue
		}
		switch {
		case r.Prerelease && strings.Contains(r.TagName, marker):
		case !r.Prerelease && !strings.Contains(r.TagName, "-"):
		default:
			continue
		}
		if best == "" || IsNewer(r.TagName, best) {
			best = r.TagName
		}
	}
	if best == "" {
		return "", fmt.Errorf("No %s release published yet.", channel)
	}
	return best, nil
}

// ArchiveName is the release archive of tag for a platform: a .zip on
// windows (goreleaser's format_overrides), a .tar.gz everywhere else.
func ArchiveName(tag, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return "nerv_" + strings.TrimPrefix(tag, "v") + "_" + goos + "_" + goarch + ext
}

// Download fetches the archive of tag for a platform and checksums.txt into
// dir, verifies the archive against its own checksums.txt line, and
// returns the archive's path.
func (c Client) Download(ctx context.Context, tag, goos, goarch, dir string) (string, error) {
	name := ArchiveName(tag, goos, goarch)
	base := c.DownloadBase + "/" + tag
	archive := filepath.Join(dir, name)
	if err := c.fetch(ctx, base+"/"+name, archive); err != nil {
		return "", err
	}
	sums := filepath.Join(dir, "checksums.txt")
	if err := c.fetch(ctx, base+"/checksums.txt", sums); err != nil {
		return "", err
	}
	if err := VerifyChecksum(archive, sums); err != nil {
		return "", err
	}
	return archive, nil
}

// VerifyChecksum checks archive against its entry in the checksums file
// (sha256sum format; a leading "*" on the name is ignored).
func VerifyChecksum(archive, checksums string) error {
	name := filepath.Base(archive)
	f, err := os.Open(checksums)
	if err != nil {
		return fmt.Errorf("checksums.txt was not downloaded.")
	}
	defer f.Close()
	want := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading checksums.txt: %w", err)
	}
	if want == "" {
		return fmt.Errorf("%s was not found in checksums.txt.", name)
	}
	data, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer data.Close()
	h := sha256.New()
	if _, err := io.Copy(h, data); err != nil {
		return fmt.Errorf("reading %s: %w", name, err)
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return fmt.Errorf("Checksum verification failed for %s.", name)
	}
	return nil
}

// Replace extracts the nerv binary from archive (a tar.gz's "nerv", or a
// windows .zip's "nerv.exe") and installs it over exe, with mode 0755: it
// is written to a temporary file next to exe first, so any failure before
// placement leaves exe untouched (the temporary file is removed). goos
// selects how the extracted binary is placed: see writeOver.
func Replace(archive, exe, goos string) error {
	if strings.HasSuffix(archive, ".zip") {
		return replaceFromZip(archive, exe, goos)
	}
	return replaceFromTarGz(archive, exe, goos)
}

// replaceFromTarGz extracts "nerv" from a tar.gz archive.
func replaceFromTarGz(archive, exe, goos string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Base(archive), err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("nerv binary not found inside %s.", filepath.Base(archive))
		}
		if err != nil {
			return fmt.Errorf("reading %s: %w", filepath.Base(archive), err)
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Clean(hdr.Name) == "nerv" {
			return writeOver(tr, exe, goos)
		}
	}
}

// replaceFromZip extracts "nerv.exe" from a windows .zip archive.
func replaceFromZip(archive, exe, goos string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Base(archive), err)
	}
	defer zr.Close()
	for _, zf := range zr.File {
		if filepath.Clean(zf.Name) != "nerv.exe" {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return fmt.Errorf("reading %s: %w", filepath.Base(archive), err)
		}
		defer rc.Close()
		return writeOver(rc, exe, goos)
	}
	return fmt.Errorf("nerv binary not found inside %s.", filepath.Base(archive))
}

// writeOver writes r to a temporary file next to exe with mode 0755, then
// places it at exe: on windows (goos == "windows") exe cannot be renamed
// or removed while it is the running process's image, so it is renamed
// aside to exe+".old" first and the temporary file takes its place (see
// placeWindows); every other goos renames the temporary file directly over
// exe, atomically, as before.
func writeOver(r io.Reader, exe, goos string) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".nerv-upgrade-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	n, err := io.Copy(tmp, io.LimitReader(r, maxBinarySize+1))
	if err != nil {
		return fmt.Errorf("extracting nerv: %w", err)
	}
	if n > maxBinarySize {
		return fmt.Errorf("extracting nerv: the binary is larger than %d bytes", maxBinarySize)
	}
	if err = tmp.Chmod(0o755); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if goos == "windows" {
		return placeWindows(tmp.Name(), exe)
	}
	return os.Rename(tmp.Name(), exe)
}

// placeWindows installs tmp over exe the way Windows allows while exe is
// the running process's image: it cannot be renamed or removed in place,
// so it is renamed aside to exe+".old" first (a stale one left by an
// earlier upgrade is removed first, best-effort, so it never blocks this
// rename), then tmp is renamed into exe's now-free name. If that second
// rename fails, the original is renamed back from exe+".old" so exe is
// never left missing; exe+".old" itself is left for CleanupStaleReplacement
// to remove on a later run, once it is no longer the running image.
func placeWindows(tmp, exe string) error {
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("renaming the running binary aside: %w", err)
	}
	if err := os.Rename(tmp, exe); err != nil {
		if restoreErr := os.Rename(old, exe); restoreErr != nil {
			return fmt.Errorf("placing the new binary failed (%w), and restoring the original failed too: %w", err, restoreErr)
		}
		return fmt.Errorf("placing the new binary: %w", err)
	}
	return nil
}

// CleanupStaleReplacement best-effort removes exe+".old": a leftover from
// an earlier windows replacement (see placeWindows) that could not be
// deleted while it was still the running process's image. Call it early
// on a later run, once that process has exited and the file is no longer
// locked. Any failure (including there being nothing to remove, or
// running on a goos where Replace never creates one) is silently ignored.
func CleanupStaleReplacement(exe string) {
	_ = os.Remove(exe + ".old")
}

func (c Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// get sends a GET and returns the response when it is 200 OK.
func (c Client) get(ctx context.Context, url, what string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "nerv-upgrade")
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", what, url, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s %s: %s", what, url, resp.Status)
	}
	return resp, nil
}

func (c Client) getJSON(ctx context.Context, url string, v any) error {
	resp, err := c.get(ctx, url, "reading")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("reading %s: %w", url, err)
	}
	return nil
}

func (c Client) fetch(ctx context.Context, url, dest string) error {
	resp, err := c.get(ctx, url, "downloading")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	return out.Close()
}
