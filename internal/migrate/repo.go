package migrate

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
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
		doc, err = translateConfig(p, o, oldData)
		if err != nil {
			return err
		}
	case newExists || prevExists:
		current := newData
		if prevExists {
			current = prevText
		}
		doc = config.Parse(string(current))
		if doc.KeyExists(alphaSection) {
			where := paths.ProjectConfigFile
			if prevExists {
				where = paths.PreviousProjectConfigFile
			}
			return refuseLegacySection(where, alphaSection, "5.0.0-alpha.1")
		}
	}
	if doc != nil {
		if tdd != nil && !doc.KeyExists("odd") {
			doc.SetScalar("odd.tdd", tdd.mode)
			doc.SetScalar("odd.test_runner", tdd.runner)
		}
		if o.TaskYAMLRepo == "" {
			// Once nerv-task owns the repository's task configuration
			// (TaskYAMLRepo set), tasks.provider no longer belongs in
			// this file: seeding it here would only have
			// moveRepoTaskYAML move it right back out on every run.
			seedTaskProvider(p, o, doc)
		}
		if err := refuseScriptTargets(doc, "deploy", paths.ProjectConfigFile); err != nil {
			return err
		}
		if err := personalKeys(p, doc, paths.ProjectConfigFile, user); err != nil {
			return err
		}
		if err := moveRepoTaskYAML(p, o, doc); err != nil {
			return err
		}
		if err := moveRepoReleaseYAML(p, o, doc); err != nil {
			return err
		}
		text := doc.String()
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
	if err := refuseProjectDeployProviders(root); err != nil {
		return err
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

// translateConfig turns a 4.2.0 project nerv.yaml into .ai-workflow/config.yaml.
// Its pipeline: and release: sections are refused: they targeted the
// release and deploy engines, which left nerv for nerv-release.
func translateConfig(p *Plan, o Options, data []byte) (*config.Document, error) {
	where := paths.ProjectConfigFile
	doc := config.Parse(string(data))
	for _, section := range []string{"pipeline", "release"} {
		if doc.KeyExists(section) {
			return nil, refuseLegacySection(".nerv/nerv.yaml", section, "4.2.0")
		}
	}
	rewriteHeader(doc)
	provider, providerSet := doc.Scalar("tasks.provider")
	if err := migrateTasks(p, doc, where); err != nil {
		return nil, err
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
	return doc, nil
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
