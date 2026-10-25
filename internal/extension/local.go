package extension

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
)

// ProviderDir is the step variable of a project provider's own folder: an
// argv element "{provider_dir}" is that folder, and "{provider_dir}/<path>"
// a file inside it (a script the provider ships). LoadProvider resolves it
// to an absolute path, refusing a path that leaves the folder (through ".."
// or a symlink); a provider nerv ships cannot use it.
const ProviderDir = "{provider_dir}"

// The project folders, under the repository's .ai-workflow/: one folder
// per extension (its extension.yaml) and one <extension>-<name> folder per
// provider (its provider.yaml, plus its scripts and procedures).
const (
	ProjectExtensionsDir = paths.ProjectDirName + "/extensions"
	ProjectProvidersDir  = paths.ProjectDirName + "/providers"
)

// projectTree is the extensions tree nerv ships merged with a repository's
// project extensions and providers: each project folder appears as a
// top-level folder next to the shipped ones, read from the repository.
type projectTree struct {
	shipped fs.FS
	// local maps a top-level folder name to its project folder.
	local map[string]projectFolder
}

type projectFolder struct {
	// dir is the absolute folder; rel its path from the repository root
	// (.ai-workflow/providers/<name>), for messages.
	dir, rel string
	// real is dir with its symlinks resolved, and repo the repository's.
	real, repo string
}

// WithProject returns shipped merged with the project extensions
// (.ai-workflow/extensions/<name>/extension.yaml) and providers
// (.ai-workflow/providers/<extension>-<name>/provider.yaml) of the
// repository at repoRoot, each checked exactly as a shipped one is, so the
// result can be read wherever the shipped tree is (LoadManifest,
// LoadProvider, the slot listings). A project provider may belong to a
// shipped or a project extension. A name nerv already ships, a folder
// outside the repository (a symlink) and a folder holding the other
// manifest are refused. With no project folder, shipped is returned as
// is.
func WithProject(shipped fs.FS, repoRoot string) (fs.FS, error) {
	if repoRoot == "" {
		return shipped, nil
	}
	repoReal, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return nil, err
	}
	if shipped == nil {
		shipped = emptyFS{}
	}
	t := &projectTree{shipped: shipped, local: map[string]projectFolder{}}
	kinds := []struct {
		rel, manifest, other string
	}{
		{ProjectExtensionsDir, ManifestFile, ProviderFile},
		{ProjectProvidersDir, ProviderFile, ManifestFile},
	}
	var extensions, providers []string
	for i, kind := range kinds {
		names, err := t.add(repoRoot, repoReal, kind.rel, kind.manifest, kind.other)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			extensions = names
		} else {
			providers = names
		}
	}
	if len(t.local) == 0 {
		if _, empty := shipped.(emptyFS); empty {
			return nil, nil
		}
		return shipped, nil
	}
	requires := map[string][]string{}
	for _, name := range extensions {
		m, err := LoadManifest(t, name)
		if err != nil {
			return nil, projectError(ProjectExtensionsDir, name, err)
		}
		requires[name] = m.Requires
	}
	isExtension := func(name string) bool {
		_, err := fs.Stat(t, name+"/"+ManifestFile)
		return ValidName(name) && err == nil
	}
	if problems := RequiresProblems(requires, isExtension); len(problems) > 0 {
		// Every problem is "<project extension>/extension.yaml: ...".
		for i := range problems {
			problems[i] = ProjectExtensionsDir + "/" + problems[i]
		}
		return nil, errors.New(strings.Join(problems, "; "))
	}
	for _, name := range providers {
		if _, _, err := LoadProvider(t, name); err != nil {
			return nil, projectError(ProjectProvidersDir, name, err)
		}
	}
	return t, nil
}

// add registers every folder of <repoRoot>/<rel> and returns their names.
func (t *projectTree) add(repoRoot, repoReal, rel, manifest, other string) ([]string, error) {
	base := filepath.Join(repoRoot, filepath.FromSlash(rel))
	entries, err := os.ReadDir(base)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		at := rel + "/" + name
		dir := filepath.Join(base, name)
		info, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", at, err)
		}
		if !info.IsDir() {
			continue
		}
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", at, err)
		}
		if !within(real, repoReal) {
			return nil, fmt.Errorf("%s: the folder resolves to %s, outside the repository", at, real)
		}
		if _, err := fs.Stat(t.shipped, name); err == nil {
			return nil, fmt.Errorf("%s: nerv already ships an extension or provider named %s; give the project's another name", at, name)
		}
		if prev, dup := t.local[name]; dup {
			return nil, fmt.Errorf("%s: %s already uses the name %s", at, prev.rel, name)
		}
		if _, err := os.Stat(filepath.Join(dir, manifest)); err != nil {
			return nil, fmt.Errorf("%s: no %s (docs/extensions.md)", at, manifest)
		}
		if _, err := os.Stat(filepath.Join(dir, other)); err == nil {
			return nil, fmt.Errorf("%s: both %s and %s: a project folder is an extension or a provider (docs/extensions.md)", at, ManifestFile, ProviderFile)
		}
		t.local[name] = projectFolder{dir: dir, rel: at, real: real, repo: repoReal}
		names = append(names, name)
	}
	return names, nil
}

// projectError names a project folder's error by its path in the
// repository: "<folder>/provider.yaml: ..." becomes
// ".ai-workflow/providers/<folder>/provider.yaml: ...".
func projectError(rel, folder string, err error) error {
	if strings.HasPrefix(err.Error(), folder+"/") {
		return fmt.Errorf("%s/%w", rel, err)
	}
	return fmt.Errorf("%s/%s: %w", rel, folder, err)
}

// ProjectFolder reports whether folder of fsys is a project folder (from
// WithProject) and, when it is, its absolute path.
func ProjectFolder(fsys fs.FS, folder string) (string, bool) {
	t, ok := fsys.(*projectTree)
	if !ok {
		return "", false
	}
	f, ok := t.local[folder]
	return f.dir, ok
}

// ProjectFolders lists the project folders of fsys (from WithProject), in
// name order: none for any other tree.
func ProjectFolders(fsys fs.FS) []string {
	t, ok := fsys.(*projectTree)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(t.local))
	for name := range t.local {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Open opens name in the project folder its first element names, or in
// the shipped tree.
func (t *projectTree) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if sub, rest, ok := t.route(name); ok {
		return sub.Open(rest)
	}
	return t.shipped.Open(name)
}

// ReadDir lists name; "." holds the shipped folders and the project ones.
func (t *projectTree) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	if sub, rest, ok := t.route(name); ok {
		return fs.ReadDir(sub, rest)
	}
	entries, err := fs.ReadDir(t.shipped, name)
	if err != nil || name != "." {
		return entries, err
	}
	for folder, f := range t.local {
		info, err := os.Stat(f.dir)
		if err != nil {
			return nil, err
		}
		entries = append(entries, renamedDir{fs.FileInfoToDirEntry(info), folder})
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}

// emptyFS is the shipped tree of a caller that has none.
type emptyFS struct{}

func (emptyFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (emptyFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "." {
		return nil, nil
	}
	return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
}

// route returns the project folder a path is inside and the rest of it.
func (t *projectTree) route(name string) (fs.FS, string, bool) {
	first, rest, _ := strings.Cut(name, "/")
	f, ok := t.local[first]
	if !ok {
		return nil, "", false
	}
	if rest == "" {
		rest = "."
	}
	return os.DirFS(f.dir), rest, true
}

// renamedDir is a project folder's entry under its top-level name.
type renamedDir struct {
	fs.DirEntry
	name string
}

func (d renamedDir) Name() string { return d.name }

// within reports whether path is dir or inside it (both cleaned, symlinks
// resolved).
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// resolveProjectPaths replaces every {provider_dir} argv element of a
// project provider's steps with the absolute path it names, and checks its
// procedure files: each must stay inside the provider folder once its
// symlinks are resolved. A provider nerv ships (dir "") cannot use
// {provider_dir}.
func (p *Provider) resolveProjectPaths(f *projectFolder) error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	for _, op := range p.operationNames() {
		o := p.Operations[op]
		at := "operations." + op
		if o.Procedure != "" && f != nil {
			if err := f.inside(o.Procedure); err != nil {
				add("%s: %v", at, err)
			}
		}
		for i := range o.Steps {
			for j := range o.Steps[i].Argv {
				a := &o.Steps[i].Argv[j]
				if !strings.Contains(a.Value, ProviderDir) {
					continue
				}
				where := fmt.Sprintf("%s[%d].argv[%d] %q", at, i, j, a.Value)
				rest, isPath := strings.CutPrefix(a.Value, ProviderDir)
				switch {
				case f == nil:
					add("%s: %s is for project providers (%s/)", where, ProviderDir, ProjectProvidersDir)
				case rest == "":
					a.Value = f.dir
				case !isPath || !strings.HasPrefix(rest, "/"):
					add("%s: %s is a whole element or the start of a path inside the provider folder (%s/<file>)", where, ProviderDir, ProviderDir)
				default:
					rel := strings.TrimPrefix(rest, "/")
					if err := f.inside(rel); err != nil {
						add("%s: %v", where, err)
						continue
					}
					a.Value = filepath.Join(f.dir, filepath.FromSlash(rel))
				}
			}
		}
	}
	if len(problems) > 0 {
		return errors.New(ProviderFile + ": " + strings.Join(problems, "; "))
	}
	return nil
}

// inside checks that rel names an existing file inside the folder, also
// once its symlinks are resolved.
func (f *projectFolder) inside(rel string) error {
	if !fs.ValidPath(rel) || rel == "." {
		return fmt.Errorf("%s leaves the provider folder", rel)
	}
	real, err := filepath.EvalSymlinks(filepath.Join(f.dir, filepath.FromSlash(rel)))
	if err != nil {
		return fmt.Errorf("%s not found in the provider folder", rel)
	}
	if !within(real, f.real) {
		return fmt.Errorf("%s resolves to %s, outside the provider folder", rel, real)
	}
	return nil
}
