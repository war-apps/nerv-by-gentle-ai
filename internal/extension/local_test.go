package extension_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/war-apps/nerv-by-gentle-ai/internal/extension"
	"github.com/war-apps/nerv-by-gentle-ai/internal/extension/extensiontest"
)

// S9: a repository's .ai-workflow/extensions/<name>/extension.yaml and
// .ai-workflow/providers/<extension>-<name>/provider.yaml join the shipped
// tree, checked with the same contract.

// shippedTree is a shipped tree with the box extension (exec slot "box")
// and its tin provider.
func shippedTree() fstest.MapFS {
	return fstest.MapFS{
		"acme/extension.yaml":    {Data: []byte(acmeExtension)},
		"acme-tin/provider.yaml": {Data: []byte(tinProvider)},
	}
}

// projectRepo writes files (paths relative to the repository) into a new
// repository and returns its root.
func projectRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	for rel, text := range files {
		path := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

// pewterProvider is an exec provider of acme's box slot that runs its own
// script from its folder.
const pewterProvider = `name: pewter
extension: acme
slot: box
contract_version: 1
runtime: exec
requires: [bash]
operations:
  ship:
    - argv: [bash, "{provider_dir}/ship.sh", "{target}"]
  check: not_implemented
label: Pewter
`

func TestWithProject_NoProjectFoldersIsTheShippedTree(t *testing.T) {
	shipped := shippedTree()
	got, err := extension.WithProject(shipped, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.(fstest.MapFS); !ok {
		t.Errorf("WithProject = %T, want the shipped tree itself", got)
	}
}

func TestWithProject_ProjectProviderOfAShippedExtension(t *testing.T) {
	repo := projectRepo(t, map[string]string{
		".ai-workflow/providers/acme-pewter/provider.yaml": pewterProvider,
		".ai-workflow/providers/acme-pewter/ship.sh":       "#!/bin/sh\n",
	})
	tree, err := extension.WithProject(shippedTree(), repo)
	if err != nil {
		t.Fatal(err)
	}
	p, ext, err := extension.LoadProvider(tree, "acme-pewter")
	if err != nil {
		t.Fatal(err)
	}
	if ext.Name != "acme" || p.Slot != "box" {
		t.Errorf("provider %s of %s.%s", p.Name, ext.Name, p.Slot)
	}
	dir := filepath.Join(repo, ".ai-workflow", "providers", "acme-pewter")
	if got := p.Operations["ship"].Steps[0].Argv[1].Value; got != filepath.Join(dir, "ship.sh") {
		t.Errorf("{provider_dir}/ship.sh = %q, want %q", got, filepath.Join(dir, "ship.sh"))
	}
	if got := p.Operations["ship"].Steps[0].Argv[2].Value; got != "{target}" {
		t.Errorf("the extension's own variables are kept: %q", got)
	}
	if got, ok := extension.ProjectFolder(tree, "acme-pewter"); !ok || got != dir {
		t.Errorf("ProjectFolder = %q, %v", got, ok)
	}
	entries, err := fs.ReadDir(tree, ".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "acme,acme-pewter,acme-tin" {
		t.Errorf("entries = %v, want the shipped and project folders in name order", names)
	}
}

func TestWithProject_ProjectExtensionAndItsProvider(t *testing.T) {
	repo := projectRepo(t, map[string]string{
		".ai-workflow/extensions/lab/extension.yaml":    string(extensiontest.Extension("lab", 0)),
		".ai-workflow/providers/lab-mice/provider.yaml": string(extensiontest.Provider("lab", "mice")),
	})
	tree, err := extension.WithProject(nil, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := extension.LoadManifest(tree, "lab"); err != nil {
		t.Error(err)
	}
	if _, _, err := extension.LoadProvider(tree, "lab-mice"); err != nil {
		t.Error(err)
	}
}

// An invalid project manifest is refused with the shipped one's message,
// named by its path in the repository.
func TestWithProject_InvalidProjectManifestsAreRefused(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"provider missing a required operation", map[string]string{
			".ai-workflow/providers/acme-pewter/provider.yaml": strings.Replace(pewterProvider, "  ship:\n    - argv: [bash, \"{provider_dir}/ship.sh\", \"{target}\"]\n", "", 1),
		}, ".ai-workflow/providers/acme-pewter/provider.yaml: operations.ship: missing (implement it or declare not_implemented)"},
		{"provider of an unknown extension", map[string]string{
			".ai-workflow/providers/nope-pewter/provider.yaml": strings.Replace(pewterProvider, "extension: acme", "extension: nope", 1),
		}, `.ai-workflow/providers/nope-pewter/provider.yaml: extension "nope" is not available (no nope/extension.yaml)`},
		{"provider in the wrong folder", map[string]string{
			".ai-workflow/providers/pewter/provider.yaml": pewterProvider,
		}, ".ai-workflow/providers/pewter/provider.yaml: a provider of extension acme named pewter lives in the folder acme-pewter"},
		{"extension with an unknown field", map[string]string{
			".ai-workflow/extensions/lab/extension.yaml": string(extensiontest.Extension("lab", 0)) + "colour: red\n",
		}, ".ai-workflow/extensions/lab/extension.yaml: yaml: unmarshal errors:"},
		{"extension requiring an extension of no tree", map[string]string{
			".ai-workflow/extensions/lab/extension.yaml": string(extensiontest.Extension("lab", 0, "acme", "omega")),
		}, ".ai-workflow/extensions/lab/extension.yaml: requires: omega is not an extension of this tree"},
		{"extensions requiring each other", map[string]string{
			".ai-workflow/extensions/lab/extension.yaml": string(extensiontest.Extension("lab", 0, "zoo")),
			".ai-workflow/extensions/zoo/extension.yaml": string(extensiontest.Extension("zoo", 0, "lab")),
		}, ".ai-workflow/extensions/lab/extension.yaml: requires cycle: lab -> zoo -> lab"},
		{"folder without its manifest", map[string]string{
			".ai-workflow/extensions/lab/README.md": "x",
		}, ".ai-workflow/extensions/lab: no extension.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := projectRepo(t, tc.files)
			_, err := extension.WithProject(shippedTree(), repo)
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("err = %v\nwant the prefix %q", err, tc.want)
			}
		})
	}
	// The same manifest shipped says the same thing, without the
	// repository path.
	shipped := shippedTree()
	shipped["acme-pewter/provider.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(pewterProvider, "  ship:\n    - argv: [bash, \"{provider_dir}/ship.sh\", \"{target}\"]\n", "", 1))}
	_, _, err := extension.LoadProvider(shipped, "acme-pewter")
	if err == nil || !strings.HasPrefix(err.Error(), "acme-pewter/provider.yaml: operations.ship: missing (implement it or declare not_implemented)") {
		t.Errorf("shipped: %v", err)
	}
}

// A project extension or provider named like a shipped one is refused.
func TestWithProject_NameCollisionsAreRefused(t *testing.T) {
	for _, tc := range []struct{ rel, want string }{
		{".ai-workflow/extensions/acme/extension.yaml", ".ai-workflow/extensions/acme: nerv already ships an extension or provider named acme; give the project's another name"},
		{".ai-workflow/providers/acme-tin/provider.yaml", ".ai-workflow/providers/acme-tin: nerv already ships an extension or provider named acme-tin; give the project's another name"},
	} {
		repo := projectRepo(t, map[string]string{tc.rel: tinProvider})
		_, err := extension.WithProject(shippedTree(), repo)
		if err == nil || err.Error() != tc.want {
			t.Errorf("%s: err = %v, want %q", tc.rel, err, tc.want)
		}
	}
}

// {provider_dir} paths must stay inside the provider folder: ".." and
// symlinks out of it are refused, and so is a missing file and a provider
// nerv ships that uses it.
func TestWithProject_ProviderDirPathsStayInTheFolder(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "evil.sh")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, arg, want string
		link            bool
	}{
		{"dot-dot", "{provider_dir}/../../../evil.sh", "../../../evil.sh leaves the provider folder", false},
		{"symlink", "{provider_dir}/link.sh", "link.sh resolves to " + outside + ", outside the provider folder", true},
		{"missing", "{provider_dir}/nope.sh", "nope.sh not found in the provider folder", false},
		{"glued", "x{provider_dir}/ship.sh", "{provider_dir} is a whole element or the start of a path inside the provider folder ({provider_dir}/<file>)", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := projectRepo(t, map[string]string{
				".ai-workflow/providers/acme-pewter/provider.yaml": strings.Replace(pewterProvider, `"{provider_dir}/ship.sh"`, `"`+tc.arg+`"`, 1),
				".ai-workflow/providers/acme-pewter/ship.sh":       "#!/bin/sh\n",
			})
			if tc.link {
				if err := os.Symlink(outside, filepath.Join(repo, ".ai-workflow", "providers", "acme-pewter", "link.sh")); err != nil {
					t.Skip("symlinks unavailable:", err)
				}
			}
			_, err := extension.WithProject(shippedTree(), repo)
			want := ".ai-workflow/providers/acme-pewter/provider.yaml: operations.ship[0].argv[1] \"" + tc.arg + "\": " + tc.want
			if err == nil || err.Error() != want {
				t.Errorf("err = %v\nwant %q", err, want)
			}
		})
	}
	shipped := shippedTree()
	shipped["acme-pewter/provider.yaml"] = &fstest.MapFile{Data: []byte(pewterProvider)}
	_, _, err := extension.LoadProvider(shipped, "acme-pewter")
	if err == nil || !strings.Contains(err.Error(), "{provider_dir} is for project providers (.ai-workflow/providers/)") {
		t.Errorf("shipped provider using {provider_dir}: %v", err)
	}
}

// A provider folder that is a symlink out of the repository is refused.
func TestWithProject_FolderOutsideTheRepositoryIsRefused(t *testing.T) {
	elsewhere := projectRepo(t, map[string]string{"acme-pewter/provider.yaml": pewterProvider, "acme-pewter/ship.sh": "#!/bin/sh\n"})
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".ai-workflow", "providers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(elsewhere, "acme-pewter"), filepath.Join(repo, ".ai-workflow", "providers", "acme-pewter")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	_, err := extension.WithProject(shippedTree(), repo)
	if err == nil || !strings.Contains(err.Error(), "outside the repository") {
		t.Errorf("err = %v, want the outside-the-repository refusal", err)
	}
}
