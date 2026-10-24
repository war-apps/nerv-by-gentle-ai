// Package artifact writes, reads and verifies the local artifact `nerv ship`
// leaves for one project's version: .nerv/artifacts/<project>/<version>/ in
// the user's repository, holding
//
//	manifest.json         the Manifest (schema nerv.ship-artifact/v1)
//	notes.md              the release notes
//	CHANGELOG.section.md  the CHANGELOG section of the release
//	outputs/              the packaged outputs (image tars, files, directories)
//	release-files/        the new contents of the files the release changes
//	                      (version files, CHANGELOG), at their clone paths
//
// The release files let deploy recreate the release commit without
// rebuilding. .nerv/artifacts/ ignores itself with a .gitignore of its own,
// so the user's .gitignore is never changed.
package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Schema is the manifest's schema identifier.
const Schema = "nerv.ship-artifact/v1"

// The files and directories of an artifact.
const (
	ManifestFile    = "manifest.json"
	NotesFile       = "notes.md"
	ChangelogFile   = "CHANGELOG.section.md"
	OutputsDir      = "outputs"
	ReleaseFilesDir = "release-files"
)

// Output kinds.
const (
	// KindImageTar is a `docker save` archive of built images.
	KindImageTar = "image-tar"
	// KindFile is one file.
	KindFile = "file"
	// KindDir is a directory tree; its SHA256 is DirDigest's.
	KindDir = "dir"
)

// Kinds are the output kinds.
var Kinds = []string{KindImageTar, KindFile, KindDir}

// ErrExists is returned by Write when the version already has an artifact
// and force is false.
var ErrExists = errors.New("artifact already exists")

// Manifest is manifest.json.
type Manifest struct {
	Schema  string `json:"schema"`
	Project string `json:"project"`
	Version string `json:"version"`
	// Channel is empty for the default channel.
	Channel string `json:"channel"`
	Tag     string `json:"tag"`
	// BaseCommit is the origin commit that was built.
	BaseCommit string    `json:"base_commit"`
	Branch     string    `json:"branch"`
	BuildMode  string    `json:"build_mode"`
	CreatedAt  time.Time `json:"created_at"`
	// Outputs are the packaged outputs, under outputs/.
	Outputs []Output `json:"outputs"`
	// Images are the references of the images saved in the image tars.
	Images []string `json:"images"`
	// ReleaseFiles are the files the release changes in the repository.
	ReleaseFiles []ReleaseFile `json:"release_files"`
}

// Output is one packaged output.
type Output struct {
	// Path is artifact-relative, under outputs/, with forward slashes.
	Path string `json:"path"`
	Kind string `json:"kind"`
	// SHA256 is the file's digest, or a directory's DirDigest, in hex.
	SHA256 string `json:"sha256"`
}

// ReleaseFile is one file the release changes.
type ReleaseFile struct {
	// Path is the file's repository-relative path, with forward slashes.
	Path string `json:"path"`
	// Stored is where its new content is, artifact-relative.
	Stored string `json:"stored"`
	SHA256 string `json:"sha256"`
}

// OutputSource is an output to copy into the artifact.
type OutputSource struct {
	// Path is the output's path under outputs/.
	Path string
	Kind string
	// From is the absolute source file or directory.
	From string
}

// ReleaseFileSource is a release file's path and new content.
type ReleaseFileSource struct {
	Path    string
	Content []byte
}

// Input is what Write records: the manifest's fields (Schema, Outputs and
// ReleaseFiles are filled by Write), the notes, the CHANGELOG section, the
// outputs and the release files.
type Input struct {
	Manifest         Manifest
	Notes            string
	ChangelogSection string
	Outputs          []OutputSource
	ReleaseFiles     []ReleaseFileSource
}

var (
	projectRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	versionRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+-]*$`)
	digestRe  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Root is <repoRoot>/.nerv/artifacts.
func Root(repoRoot string) string { return filepath.Join(repoRoot, ".nerv", "artifacts") }

// Dir is the artifact directory of project's version.
func Dir(repoRoot, project, version string) string {
	return filepath.Join(Root(repoRoot), project, version)
}

// Write writes the artifact of in under repoRoot and returns its directory.
// The artifact is assembled in a temporary sibling directory and renamed
// into place, so a failed write leaves nothing behind. An existing artifact
// for the version is refused with ErrExists unless force, which replaces it.
func Write(repoRoot string, in Input, force bool) (string, error) {
	m := in.Manifest
	if !projectRe.MatchString(m.Project) {
		return "", fmt.Errorf("artifact: project %q is not a valid name", m.Project)
	}
	if !versionRe.MatchString(m.Version) || strings.Contains(m.Version, "..") {
		return "", fmt.Errorf("artifact: version %q is not a valid version", m.Version)
	}
	if err := checkSources(in); err != nil {
		return "", err
	}
	dir := Dir(repoRoot, m.Project, m.Version)
	if err := prepare(repoRoot, filepath.Dir(dir)); err != nil {
		return "", err
	}
	if _, err := os.Lstat(dir); err == nil && !force {
		return "", fmt.Errorf("%w: %s (pass --force to replace it)", ErrExists, dir)
	}

	tmp, err := os.MkdirTemp(filepath.Dir(dir), ".tmp-"+m.Version+"-")
	if err != nil {
		return "", err
	}
	done := false
	defer func() {
		if !done {
			os.RemoveAll(tmp)
		}
	}()
	m.Schema = Schema
	m.Outputs, m.ReleaseFiles = []Output{}, []ReleaseFile{}
	if m.Images == nil {
		m.Images = []string{}
	}
	for _, f := range []struct{ name, content string }{{NotesFile, in.Notes}, {ChangelogFile, in.ChangelogSection}} {
		if err := os.WriteFile(filepath.Join(tmp, f.name), []byte(f.content), 0o644); err != nil {
			return "", err
		}
	}
	for _, o := range in.Outputs {
		rel := path.Join(OutputsDir, o.Path)
		digest, err := copyOutput(o, filepath.Join(tmp, filepath.FromSlash(rel)))
		if err != nil {
			return "", fmt.Errorf("artifact: output %s: %w", o.Path, err)
		}
		m.Outputs = append(m.Outputs, Output{Path: rel, Kind: o.Kind, SHA256: digest})
	}
	for _, f := range in.ReleaseFiles {
		stored := path.Join(ReleaseFilesDir, f.Path)
		dst := filepath.Join(tmp, filepath.FromSlash(stored))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(dst, f.Content, 0o644); err != nil {
			return "", err
		}
		h := sha256.Sum256(f.Content)
		m.ReleaseFiles = append(m.ReleaseFiles, ReleaseFile{Path: f.Path, Stored: stored, SHA256: hex.EncodeToString(h[:])})
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmp, ManifestFile), append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	if force {
		if err := os.RemoveAll(dir); err != nil {
			return "", fmt.Errorf("artifact: replacing %s: %w", dir, err)
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		return "", err
	}
	done = true
	return dir, nil
}

// checkSources refuses an input whose paths are unsafe or repeated, or
// whose kind is unknown, before anything is written.
func checkSources(in Input) error {
	seen := map[string]bool{}
	for _, o := range in.Outputs {
		if !localPath(o.Path) {
			return fmt.Errorf("artifact: output path %q must be a relative path inside outputs/", o.Path)
		}
		if seen[o.Path] {
			return fmt.Errorf("artifact: output path %q given twice", o.Path)
		}
		seen[o.Path] = true
		if !slices.Contains(Kinds, o.Kind) {
			return fmt.Errorf("artifact: output %s: unknown kind %q (want %s)", o.Path, o.Kind, strings.Join(Kinds, ", "))
		}
	}
	files := map[string]bool{}
	for _, f := range in.ReleaseFiles {
		if !localPath(f.Path) {
			return fmt.Errorf("artifact: release file %q must be a repository-relative path", f.Path)
		}
		if files[f.Path] {
			return fmt.Errorf("artifact: release file %q given twice", f.Path)
		}
		files[f.Path] = true
	}
	return nil
}

// localPath reports a clean, relative, forward-slash path that stays below
// its root.
func localPath(p string) bool {
	return p != "" && !strings.Contains(p, `\`) && path.Clean(p) == p && filepath.IsLocal(filepath.FromSlash(p))
}

// copyOutput copies o to dst and returns its digest.
func copyOutput(o OutputSource, dst string) (string, error) {
	info, err := os.Lstat(o.From)
	if err != nil {
		return "", err
	}
	if o.Kind == KindDir {
		if !info.IsDir() {
			return "", fmt.Errorf("%s is not a directory", o.From)
		}
		if err := copyTree(o.From, dst); err != nil {
			return "", err
		}
		return DirDigest(dst)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", o.From)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	return copyFile(o.From, dst)
}

// copyTree copies the regular files and directories under src; a symlink
// or any other file type is refused.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type().IsRegular():
			_, err := copyFile(p, target)
			return err
		}
		return fmt.Errorf("%s is not a regular file or directory", p)
	})
}

// copyFile copies src to dst and returns the content's sha256.
func copyFile(src, dst string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fileDigest is the sha256 of the file at p.
func fileDigest(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// DirDigest is a directory's digest: the sha256 of its sha256sum listing,
// one "<sha256>  <path>\n" line per regular file, paths relative and with
// forward slashes, sorted by path. Anything but regular files and
// directories is an error.
func DirDigest(dir string) (string, error) {
	var lines []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return nil
		case !d.Type().IsRegular():
			return fmt.Errorf("%s is not a regular file or directory", p)
		}
		digest, err := fileDigest(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		lines = append(lines, digest+"  "+filepath.ToSlash(rel)+"\n")
		return nil
	})
	if err != nil {
		return "", err
	}
	slices.SortFunc(lines, func(a, b string) int { return strings.Compare(a[66:], b[66:]) })
	h := sha256.Sum256([]byte(strings.Join(lines, "")))
	return hex.EncodeToString(h[:]), nil
}

// Read reads and checks the manifest of the artifact at dir: its schema,
// and that every output and release file stays inside the artifact.
func Read(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		return nil, fmt.Errorf("artifact: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("artifact: %s: %w", filepath.Join(dir, ManifestFile), err)
	}
	if m.Schema != Schema {
		return nil, fmt.Errorf("artifact: %s: schema %q, want %q", dir, m.Schema, Schema)
	}
	for _, o := range m.Outputs {
		rest, ok := strings.CutPrefix(o.Path, OutputsDir+"/")
		if !ok || !localPath(rest) {
			return nil, fmt.Errorf("artifact: %s: output path %q is not inside %s/", dir, o.Path, OutputsDir)
		}
		if !slices.Contains(Kinds, o.Kind) {
			return nil, fmt.Errorf("artifact: %s: output %s: unknown kind %q", dir, o.Path, o.Kind)
		}
		if !digestRe.MatchString(o.SHA256) {
			return nil, fmt.Errorf("artifact: %s: output %s: sha256 %q is not a hex digest", dir, o.Path, o.SHA256)
		}
	}
	for _, f := range m.ReleaseFiles {
		rest, ok := strings.CutPrefix(f.Stored, ReleaseFilesDir+"/")
		if !localPath(f.Path) || !ok || !localPath(rest) {
			return nil, fmt.Errorf("artifact: %s: release file %q (stored %q) is not a safe relative path", dir, f.Path, f.Stored)
		}
		if !digestRe.MatchString(f.SHA256) {
			return nil, fmt.Errorf("artifact: %s: release file %s: sha256 %q is not a hex digest", dir, f.Path, f.SHA256)
		}
	}
	return &m, nil
}

// Verify reads the artifact at dir and checks the sha256 of every output
// and release file.
func Verify(dir string) (*Manifest, error) {
	m, err := Read(dir)
	if err != nil {
		return nil, err
	}
	check := func(rel, kind, want string) error {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		var got string
		var err error
		if kind == KindDir {
			got, err = DirDigest(p)
		} else {
			got, err = fileDigest(p)
		}
		switch {
		case err != nil:
			return fmt.Errorf("artifact: %s: %w", rel, err)
		case got != want:
			return fmt.Errorf("artifact: %s: sha256 %s, the manifest records %s", rel, got, want)
		}
		return nil
	}
	for _, o := range m.Outputs {
		if err := check(o.Path, o.Kind, o.SHA256); err != nil {
			return nil, err
		}
	}
	for _, f := range m.ReleaseFiles {
		if err := check(f.Stored, KindFile, f.SHA256); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// prepare creates dir under the repository's .nerv/artifacts, which gets a
// .gitignore of "*" when it has none, after checking that, symlinks
// resolved, nothing on the way leads outside the repository.
func prepare(repoRoot, dir string) error {
	realRepo, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return err
	}
	for _, p := range []string{Root(repoRoot), dir} {
		existing := p
		for {
			if _, err := os.Lstat(existing); err == nil {
				break
			}
			parent := filepath.Dir(existing)
			if parent == existing {
				break
			}
			existing = parent
		}
		real, err := filepath.EvalSymlinks(existing)
		if err != nil {
			return err
		}
		if rel, err := filepath.Rel(realRepo, real); err != nil || !(rel == "." || filepath.IsLocal(rel)) {
			return fmt.Errorf("artifact: %s resolves to %s, outside the repository: refusing to write there", existing, real)
		}
		if err := os.MkdirAll(p, 0o755); err != nil {
			return err
		}
	}
	ignore := filepath.Join(Root(repoRoot), ".gitignore")
	if _, err := os.Lstat(ignore); errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(ignore, []byte("*\n"), 0o644)
	}
	return nil
}
