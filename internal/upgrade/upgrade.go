// Package upgrade replaces the running nerv binary with a newer GitHub
// release: it resolves the newest release of a channel, downloads the
// platform's archive and checksums.txt, verifies the archive's SHA-256,
// extracts the nerv binary and renames it over the running one. It mirrors
// scripts/install.sh: the same release lookup, asset names, checksum check
// and supported platforms.
package upgrade

import (
	"archive/tar"
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
// binary for (only linux|darwin and amd64|arm64).
func CheckPlatform(goos, goarch string) error {
	switch goos {
	case "linux", "darwin":
	default:
		return &refusal.Error{Err: fmt.Errorf("Unsupported OS: %s. Only Linux and macOS have prebuilt nerv binaries.\n%s", goos, goInstallHint)}
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

// ArchiveName is the release archive of tag for a platform.
func ArchiveName(tag, goos, goarch string) string {
	return "nerv_" + strings.TrimPrefix(tag, "v") + "_" + goos + "_" + goarch + ".tar.gz"
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

// Replace extracts the nerv binary from archive and renames it over exe
// atomically, with mode 0755: it is written to a temporary file next to exe
// first, so any failure leaves exe untouched (the temporary file is
// removed).
func Replace(archive, exe string) (err error) {
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
			return writeOver(tr, exe)
		}
	}
}

// writeOver writes r to a temporary file next to exe and renames it over
// exe with mode 0755.
func writeOver(r io.Reader, exe string) (err error) {
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
	return os.Rename(tmp.Name(), exe)
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
