// Package plugin materializes the embedded plugin tree onto disk and writes
// the directory-marketplace manifest that registers it in Claude Code.
package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	filePerm = 0o644
	dirPerm  = 0o755

	pluginManifestPath     = ".claude-plugin/plugin.json"
	marketplaceManifestRel = ".claude-plugin/marketplace.json"
)

// Result reports what Materialize did, as paths relative to dest.
type Result struct {
	// Written lists files that were created or whose content changed.
	Written []string
	// Unchanged lists files whose on-disk bytes already matched the source.
	Unchanged []string
	// Removed lists stale files under dest/plugin that are not present in
	// src anymore.
	Removed []string
}

type marketplaceManifest struct {
	Name    string              `json:"name"`
	Owner   marketplaceOwner    `json:"owner"`
	Plugins []marketplacePlugin `json:"plugins"`
}

type marketplaceOwner struct {
	Name string `json:"name"`
}

type marketplacePlugin struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description"`
}

// Materialize writes the whole tree from src under dest/plugin, byte-exact,
// and writes dest/.claude-plugin/marketplace.json registering it as a
// directory marketplace named "nerv". It is idempotent: a file whose bytes
// already match the source is left untouched, and files under dest/plugin
// that are no longer present in src are removed. Materialize never writes
// or removes anything outside dest/plugin and dest/.claude-plugin.
func Materialize(src fs.FS, dest string) (Result, error) {
	var result Result

	if err := ensureDestIsUsable(dest); err != nil {
		return result, err
	}

	destPluginRoot := filepath.Join(dest, "plugin")
	if err := os.MkdirAll(destPluginRoot, dirPerm); err != nil {
		return result, fmt.Errorf("creating %s: %w", destPluginRoot, err)
	}

	knownFiles := make(map[string]struct{})

	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walking source at %q: %w", path, walkErr)
		}
		if path == "." {
			return nil
		}
		if err := checkSafeRelPath(path); err != nil {
			return err
		}

		destPath := filepath.Join(destPluginRoot, filepath.FromSlash(path))
		if err := checkWithinRoot(destPluginRoot, destPath); err != nil {
			return err
		}

		if d.IsDir() {
			if err := os.MkdirAll(destPath, dirPerm); err != nil {
				return fmt.Errorf("creating directory %s: %w", destPath, err)
			}
			return nil
		}

		data, err := fs.ReadFile(src, path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		relDest := "plugin/" + path
		knownFiles[relDest] = struct{}{}

		changed, err := writeIfChanged(destPath, data)
		if err != nil {
			return err
		}
		if changed {
			result.Written = append(result.Written, relDest)
		} else {
			result.Unchanged = append(result.Unchanged, relDest)
		}
		return nil
	})
	if err != nil {
		return result, err
	}

	removed, err := removeStaleFiles(destPluginRoot, knownFiles)
	if err != nil {
		return result, err
	}
	result.Removed = removed

	description, err := readPluginDescription(src)
	if err != nil {
		return result, err
	}

	if err := writeMarketplaceManifest(dest, description, &result); err != nil {
		return result, err
	}

	sort.Strings(result.Written)
	sort.Strings(result.Unchanged)
	sort.Strings(result.Removed)

	return result, nil
}

func ensureDestIsUsable(dest string) error {
	info, err := os.Stat(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("checking dest %s: %w", dest, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("dest %s exists and is not a directory", dest)
	}
	return nil
}

// checkSafeRelPath refuses any fs entry whose path escapes the source root,
// either through a ".." segment or an absolute path.
func checkSafeRelPath(path string) error {
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return fmt.Errorf("refusing absolute path from source filesystem: %q", path)
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == ".." {
			return fmt.Errorf("refusing path traversal from source filesystem: %q", path)
		}
	}
	return nil
}

// checkWithinRoot is a defense-in-depth guard ensuring the resolved
// destination path still lives under root.
func checkWithinRoot(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("resolving %s relative to %s: %w", target, root, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to write outside %s: %s", root, target)
	}
	return nil
}

// writeIfChanged writes data to destPath only when the existing content
// differs, creating parent directories as needed. It reports whether it
// wrote a new file or new content.
func writeIfChanged(destPath string, data []byte) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(destPath), dirPerm); err != nil {
		return false, fmt.Errorf("creating directory for %s: %w", destPath, err)
	}

	existing, err := os.ReadFile(destPath)
	if err == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("reading %s: %w", destPath, err)
	}

	if err := os.WriteFile(destPath, data, filePerm); err != nil {
		return false, fmt.Errorf("writing %s: %w", destPath, err)
	}
	return true, nil
}

// removeStaleFiles deletes files under root that are not present in
// knownFiles (keyed as "plugin/<relative path>"), then removes any
// directories left empty by that cleanup.
func removeStaleFiles(root string, knownFiles map[string]struct{}) ([]string, error) {
	var removed []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(filepath.Dir(root), path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)

		if _, ok := knownFiles[relSlash]; ok {
			return nil
		}

		if err := os.Remove(path); err != nil {
			return fmt.Errorf("removing stale file %s: %w", path, err)
		}
		removed = append(removed, relSlash)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := removeEmptyDirs(root); err != nil {
		return nil, err
	}

	return removed, nil
}

// removeEmptyDirs prunes directories under root left empty after stale-file
// removal, without ever removing root itself.
func removeEmptyDirs(root string) error {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Remove deepest directories first so parents become empty in turn.
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("reading directory %s: %w", dir, err)
		}
		if len(entries) == 0 {
			if err := os.Remove(dir); err != nil {
				return fmt.Errorf("removing empty directory %s: %w", dir, err)
			}
		}
	}
	return nil
}

func readPluginDescription(src fs.FS) (string, error) {
	data, err := fs.ReadFile(src, pluginManifestPath)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", pluginManifestPath, err)
	}
	var manifest struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("parsing %s: %w", pluginManifestPath, err)
	}
	return manifest.Description, nil
}

func writeMarketplaceManifest(dest, description string, result *Result) error {
	manifest := marketplaceManifest{
		Name:  "nerv",
		Owner: marketplaceOwner{Name: "Walter Rodriguez"},
		Plugins: []marketplacePlugin{
			{
				Name:        "nerv",
				Source:      "./plugin",
				Description: description,
			},
		},
	}

	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding marketplace.json: %w", err)
	}
	encoded = append(encoded, '\n')

	claudePluginDir := filepath.Join(dest, ".claude-plugin")
	if err := os.MkdirAll(claudePluginDir, dirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", claudePluginDir, err)
	}

	destPath := filepath.Join(claudePluginDir, "marketplace.json")
	changed, err := writeIfChanged(destPath, encoded)
	if err != nil {
		return err
	}
	if changed {
		result.Written = append(result.Written, marketplaceManifestRel)
	} else {
		result.Unchanged = append(result.Unchanged, marketplaceManifestRel)
	}
	return nil
}
