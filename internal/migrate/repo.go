package migrate

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/deployconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/releaseplan"
	"github.com/war-apps/nerv-by-gentle-ai/internal/versioning"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

// readOptional reads path; a missing file is (nil, false, nil).
func readOptional(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, refuse("%s is not a regular file: move it by hand", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// migratedPath is where an old file is kept once moved.
func migratedPath(old string, o Options) string {
	dst := old + MigratedSuffix
	if _, err := os.Lstat(dst); err == nil {
		dst += "-" + o.Now.Format("20060102-150405")
	}
	return dst
}

// rewriteHeader updates the comment lines that name the old files.
func rewriteHeader(doc *config.Document) {
	lines := strings.SplitAfter(doc.String(), "\n")
	for i, l := range lines {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		l = strings.ReplaceAll(l, "~/.claude/nerv/nerv.yaml", "the user config.yaml")
		l = strings.ReplaceAll(l, ".nerv/nerv.yaml", paths.ProjectConfigFile)
		l = strings.ReplaceAll(l, paths.PreviousProjectConfigFile, paths.ProjectConfigFile)
		l = strings.ReplaceAll(l, "ai-workflow/config.yml", "ai-workflow/config.yaml")
		lines[i] = l
	}
	*doc = *config.Parse(strings.Join(lines, ""))
}

// planUser plans the user configuration: the 4.2.0 ~/.claude/nerv/nerv.yaml
// becomes config.yaml (task rules applied) and is kept as .migrated, and so
// does the previous config.yml.
func planUser(p *Plan, o Options) (*userState, error) {
	pp := paths.Resolve(o.Home)
	u := &userState{path: pp.UserConfig}
	newData, newExists, err := readOptional(pp.UserConfig)
	if err != nil {
		return nil, err
	}
	prevData, prevExists, err := readOptional(pp.PreviousUserConfig)
	if err != nil {
		return nil, err
	}
	if prevExists {
		// The previous config.yml is today's configuration under its old
		// name: it is the one the 4.2.0 file is compared with, and it moves.
		doc := config.Parse(string(prevData))
		rewriteHeader(doc)
		if newExists && string(newData) != doc.String() {
			return nil, refuse("%s and %s both exist: nerv migrate does not merge two user configurations; move the settings of the old one into the new one by hand (nerv configure --set), or remove %s and run nerv migrate --user again", pp.PreviousUserConfig, pp.UserConfig, pp.PreviousUserConfig)
		}
		to := migratedPath(pp.PreviousUserConfig, o)
		p.Changes = append(p.Changes, Change{Kind: Rename, Path: pp.PreviousUserConfig, To: to, Old: prevData, User: true,
			Why: "a copy of the old user configuration; your settings move to " + pp.UserConfig})
		newData, newExists = []byte(doc.String()), true
		u.moved = true
	}
	oldData, oldExists, err := readOptional(pp.LegacyUserConfig)
	if err != nil {
		return nil, err
	}
	switch {
	case oldExists:
		doc := config.Parse(string(oldData))
		rewriteHeader(doc)
		if err := migrateTasks(p, doc, "user config"); err != nil {
			return nil, err
		}
		if newExists && string(newData) != doc.String() {
			return nil, refuse("%s and %s both exist: nerv migrate does not merge two user configurations; move the settings of the old one into the new one by hand (nerv configure --set), or remove %s and run nerv migrate --user again", pp.LegacyUserConfig, pp.UserConfig, pp.UserConfig)
		}
		u.doc, u.exists, u.original = doc, newExists && !u.moved, newData
		to := migratedPath(pp.LegacyUserConfig, o)
		p.Changes = append(p.Changes, Change{Kind: Rename, Path: pp.LegacyUserConfig, To: to, Old: oldData, User: true,
			Why: "a copy of the old user configuration; your settings move to " + pp.UserConfig})
	case newExists:
		u.doc, u.exists, u.original = config.Parse(string(newData)), !u.moved, newData
	default:
		u.doc = config.Parse("")
	}
	return u, nil
}

// legacyCommandsDir is where 4.2.0's /task:* command copies lived.
func legacyCommandsDir(home string) string {
	return filepath.Join(home, ".claude", "commands", "task")
}

// planRepo plans the repository at o.RepoRoot.
func planRepo(p *Plan, o Options, user *userState) error {
	root := o.RepoRoot
	oldPath, newPath := paths.LegacyProjectConfig(root), paths.ProjectConfig(root)
	oldData, oldExists, err := readOptional(oldPath)
	if err != nil {
		return err
	}
	newData, newExists, err := readOptional(newPath)
	if err != nil {
		return err
	}
	prevPath := paths.PreviousProjectConfig(root)
	prevData, prevExists, err := readOptional(prevPath)
	if err != nil {
		return err
	}
	var prevText []byte
	if prevExists {
		// The previous .ai-workflow.yml is today's configuration under its
		// old name: it moves into the folder, converted as if it were there.
		if newExists && string(newData) != string(prevData) {
			return refuse("%s and %s both exist: nerv migrate does not merge them; merge the old one into %s by hand, or remove %s and run nerv migrate again", prevPath, newPath, paths.ProjectConfigFile, paths.PreviousProjectConfigFile)
		}
		if oldExists {
			return refuse("%s and %s both exist: nerv migrate does not merge them; merge the old one into %s by hand, or remove %s and run nerv migrate again", oldPath, prevPath, paths.PreviousProjectConfigFile, ".nerv/nerv.yaml")
		}
		doc := config.Parse(string(prevData))
		rewriteHeader(doc)
		prevText = []byte(doc.String())
	}
	tdd, err := planODD(p, o)
	if err != nil {
		return err
	}
	var doc *config.Document
	switch {
	case oldExists:
		doc, err = translateConfig(p, o, oldData, user)
		if err != nil {
			return err
		}
	case newExists || prevExists:
		current := newData
		if prevExists {
			current = prevText
		}
		doc = config.Parse(string(current))
		if doc.KeyExists(releaseconfig.AlphaSectionKey) {
			// The 5.0.0-alpha.1 ship: section is today's release: under its
			// old name, with the keys of before the version and build slots.
			where := paths.ProjectConfigFile
			if prevExists {
				where = paths.PreviousProjectConfigFile
			}
			if err := migrateShip(p, doc, where); err != nil {
				return err
			}
			p.Plugins = append(p.Plugins, "release")
		} else if legacy := releaseconfig.LegacyKeys(current); len(legacy) > 0 {
			// A 4.2.0 release: section has today's key: its 4.2.0 keys
			// tell it apart.
			p.note("%s: the release: section is the 4.2.0 one (%s)", paths.ProjectConfigFile, strings.Join(legacy, ", "))
			if doc, _, err = migrateRelease(p, o, doc); err != nil {
				return err
			}
			p.Plugins = append(p.Plugins, "release")
		} else if doc.KeyExists(releaseconfig.SectionKey) {
			if err := moveReleaseKeys(p, doc); err != nil {
				return err
			}
		}
	}
	if doc != nil {
		if tdd != nil && !doc.KeyExists("odd") {
			doc.SetScalar("odd.tdd", tdd.mode)
			doc.SetScalar("odd.test_runner", tdd.runner)
		}
		seedTaskProvider(p, o, doc)
		if err := personalKeys(p, doc, paths.ProjectConfigFile, o.ExtensionsFS, user); err != nil {
			return err
		}
		text := doc.String()
		if err := checkLoads(text); err != nil {
			return err
		}
		switch {
		case oldExists && newExists && string(newData) != text:
			return refuse("%s and %s both exist: nerv migrate does not merge them; merge the old one into %s by hand, or remove %s and run nerv migrate again", oldPath, newPath, paths.ProjectConfigFile, paths.ProjectConfigFile)
		case !newExists:
			from := ".nerv/nerv.yaml"
			if prevExists {
				from = paths.PreviousProjectConfigFile
			}
			p.Changes = append(p.Changes, Change{Kind: Write, Path: newPath, New: []byte(text), Why: "the new project configuration, from " + from})
		case string(newData) != text:
			p.Changes = append(p.Changes, Change{Kind: Write, Path: newPath, Old: newData, New: []byte(text), Why: "converted to this version's sections"})
		}
		if prevExists {
			p.Changes = append(p.Changes, Change{Kind: Remove, Path: prevPath, Old: prevData,
				Why: "replaced by " + paths.ProjectConfigFile + " (a backup is kept under .nerv/backups/)"})
		}
		if oldExists {
			to := migratedPath(oldPath, o)
			p.Changes = append(p.Changes, Change{Kind: Rename, Path: oldPath, To: to, Old: oldData,
				Why: "a copy of the old configuration, so an older nerv does not read it"})
		}
	} else if tdd != nil {
		p.warn("odd.tdd: the repository has no %s to record strict_tdd in; run nerv configure --init, then nerv migrate again", paths.ProjectConfigFile)
	}
	ignore := filepath.Join(paths.RuntimeDir(root), ".gitignore")
	if _, err := os.Lstat(ignore); errors.Is(err, fs.ErrNotExist) {
		if _, err := os.Stat(paths.RuntimeDir(root)); err == nil || len(p.Changes) > 0 {
			p.Changes = append(p.Changes, Change{Kind: Write, Path: ignore, New: []byte("*\n"), Why: "keeps nerv's local files out of git"})
		}
	}
	if o.Home != "" {
		if entries, err := os.ReadDir(legacyCommandsDir(o.Home)); err == nil && len(entries) > 0 {
			p.warn("%s: old /task:* copies shadow the task extension's commands; nerv install reports them and removes them when you confirm", legacyCommandsDir(o.Home))
		}
	}
	return nil
}

// checkLoads refuses a migrated project file that today's strict loaders
// of the release: and deploy: sections reject.
func checkLoads(text string) error {
	dir, err := os.MkdirTemp("", "nerv-migrate-check-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.MkdirAll(filepath.Dir(paths.ProjectConfig(dir)), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(paths.ProjectConfig(dir), []byte(text), 0o600); err != nil {
		return err
	}
	if _, err := releaseconfig.Load(dir); err != nil && !errors.Is(err, releaseconfig.ErrNotConfigured) {
		return refuse("the migrated %s does not load: %v", paths.ProjectConfigFile, strings.ReplaceAll(err.Error(), dir+string(filepath.Separator), ""))
	}
	if _, err := deployconfig.Load(dir); err != nil && !errors.Is(err, deployconfig.ErrNotConfigured) {
		return refuse("the migrated %s does not load: %v", paths.ProjectConfigFile, strings.ReplaceAll(err.Error(), dir+string(filepath.Separator), ""))
	}
	return nil
}

// translateConfig turns a 4.2.0 project nerv.yaml into .ai-workflow/config.yaml.
func translateConfig(p *Plan, o Options, data []byte, user *userState) (*config.Document, error) {
	where := paths.ProjectConfigFile
	doc := config.Parse(string(data))
	rewriteHeader(doc)
	provider, providerSet := doc.Scalar("tasks.provider")
	if err := migrateTasks(p, doc, where); err != nil {
		return nil, err
	}
	var dep *deployTranslation
	hadPipeline := doc.KeyExists("pipeline")
	if hadPipeline {
		var err error
		if dep, err = translatePipeline(p, []byte(doc.String()), o.ExtensionsFS, user); err != nil {
			return nil, err
		}
		doc.SetBlock("pipeline", "")
		p.note("pipeline: removed (nerv pipeline was removed)")
	}
	hadRelease := doc.KeyExists("release")
	githubRelease := false
	if hadRelease {
		var err error
		if doc, githubRelease, err = migrateRelease(p, o, doc); err != nil {
			return nil, err
		}
	}
	if (dep != nil || githubRelease) && !doc.KeyExists("deploy") {
		if dep == nil {
			dep = &deployTranslation{}
		}
		switch {
		case isGitHub(o.OriginURL):
			dep.host = githubHost
		case o.OriginURL == "":
			p.warn("deploy.host: the repository has no origin, so no git host is written: set deploy.host when it has one")
		default:
			p.warn("deploy.host: origin %s is not GitHub and nerv ships no other git host provider yet: no tag release is created on the host", o.OriginURL)
		}
		block, err := deployBlock(dep)
		if err != nil {
			return nil, err
		}
		doc.SetBlock("deploy", block)
		p.note("deploy: written (git writes, the tag and the host release are nerv deploy's now)")
	}

	// The plugins the old configuration used, in install order.
	effective, from := provider, "the project"
	if !providerSet {
		effective, from = userProvider(o), "the user config"
	}
	if effective == "" {
		effective, from = teamwork, "the 4.2.0 default"
	}
	if effective == teamwork {
		p.Plugins = append(p.Plugins, "task", "task-teamwork")
		p.note("tasks.provider: teamwork (%s): the task extension and its task-teamwork provider", from)
	}
	if hadRelease || dep != nil {
		// release brings its deploy providers with it (providers: bundled).
		p.Plugins = append(p.Plugins, "release")
	}
	p.Plugins = slices.Compact(p.Plugins)
	return doc, nil
}

// migrateRelease converts the 4.2.0 release: section of doc into today's
// and imports the repository's semantic-release configuration into it. It
// reports whether that configuration released on GitHub.
func migrateRelease(p *Plan, o Options, doc *config.Document) (*config.Document, bool, error) {
	release, err := translateRelease(p, []byte(doc.String()), o.RepoRoot)
	if err != nil {
		return nil, false, err
	}
	doc.SetBlock("release", release.block)
	githubRelease := false
	imported, err := versioning.Import(o.RepoRoot)
	var manual *versioning.ManualMigrationError
	switch {
	case errors.As(err, &manual):
		p.warn("%v; write release.providers.conventional-commits' branches and release_rules from it (see /release:setup)", err)
	case errors.Is(err, versioning.ErrNoSemanticReleaseConfig):
		p.note("no semantic-release configuration: the version engine's default branches apply (main; develop alpha; release/* rc on beta; hotfix/* beta)")
	case err != nil:
		p.warn("the semantic-release configuration cannot be imported: %v", err)
	default:
		engine, err := releaseplan.EngineYAML(imported.Config)
		if err != nil {
			return nil, false, refuse("importing %s: %v", imported.Source, err)
		}
		text, notes, err := releaseconfig.ImportSection([]byte(doc.String()), releaseconfig.EngineProvider, engine, false)
		if err != nil {
			return nil, false, refuse("importing %s into the release: section: %v", imported.Source, err)
		}
		doc = config.Parse(text)
		for _, n := range notes {
			p.note("%s", n)
		}
		for _, w := range imported.Warnings {
			if w.Where == "@semantic-release/github" {
				githubRelease = true
			}
			p.warn("semantic-release %s: %s", imported.Source, w)
		}
		p.note("the semantic-release configuration of %s was imported into release.providers.conventional-commits; it is no longer used", imported.Source)
	}
	return doc, githubRelease, nil
}

// userProvider is the tasks.provider of the user's configuration (today's
// seed defaults.tasks.provider, else its older tasks.provider, the
// previous config.yml's or the 4.2.0 one's), or "".
func userProvider(o Options) string {
	if o.Home == "" {
		return ""
	}
	if wc, err := workflowconfig.Load(o.Home, ""); err == nil && wc.UserExists {
		if d := workflowconfig.Defaults(wc.User); d != nil {
			if v, ok := d.Scalar("tasks.provider"); ok {
				return v
			}
		}
		v, _ := wc.User.Scalar("tasks.provider")
		return v
	}
	if data, ok, _ := readOptional(paths.Resolve(o.Home).PreviousUserConfig); ok {
		v, _ := config.Parse(string(data)).Scalar("tasks.provider")
		return v
	}
	if data, ok, _ := readOptional(paths.Resolve(o.Home).LegacyUserConfig); ok {
		v, _ := config.Parse(string(data)).Scalar("tasks.provider")
		return v
	}
	return ""
}
