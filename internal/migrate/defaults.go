package migrate

import (
	"io/fs"
	"slices"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"github.com/war-apps/nerv-by-gentle-ai/internal/workflowconfig"
)

// moveUserDefaults moves the user configuration's run-time keys of the
// extensions' sections (tasks, release, deploy) under defaults.<section>:
// at run time those sections are read from the project only, and a
// default is what the setup flows copy into a project. The personal keys
// (workflowconfig.Personal: the scope: user provider keys of fsys's
// manifests, the timer store) stay where they are. A section whose
// defaults are already set is reported and left alone.
func moveUserDefaults(p *Plan, u *userState, fsys fs.FS) error {
	// The task conversions first, so a removed provider is dropped rather
	// than moved into a seed; on a converted file they change nothing.
	if err := migrateTasks(p, u.doc, "user config"); err != nil {
		return err
	}
	personal, err := workflowconfig.LoadPersonal(fsys)
	if err != nil {
		return err
	}
	runTime, err := workflowconfig.RunTimeUserKeys(u.doc, personal)
	if err != nil {
		return refuse("the user config %s is not valid YAML: %v", u.path, err)
	}
	for _, section := range workflowconfig.Sections {
		keys := slices.DeleteFunc(slices.Clone(runTime), func(k string) bool { return !strings.HasPrefix(k, section+".") })
		if len(keys) == 0 {
			continue
		}
		list := strings.Join(keys, ", ")
		block, found := u.doc.Block(section)
		if !found {
			p.warn("user config: %s: no longer read at run time; move it under defaults.%s by hand", list, section)
			continue
		}
		if d := workflowconfig.Defaults(u.doc); d != nil && d.KeyExists(section) {
			p.warn("user config: %s: kept, defaults.%s is already set; merge it into defaults.%s by hand", list, section, section)
			continue
		}
		moved, kept := config.Parse(block), config.Parse(block)
		if err := workflowconfig.Strip(moved, section, personal.Has); err != nil {
			return err
		}
		if err := workflowconfig.Strip(kept, section, func(k string) bool { return !personal.Has(k) }); err != nil {
			return err
		}
		keptBlock, _ := kept.Block(section)
		u.doc.SetBlock(section, keptBlock)
		movedBlock, _ := moved.Block(section)
		appendDefaults(u.doc, movedBlock)
		p.note("user config: %s moved under defaults.%s: a default that the setup commands copy into a project, never read at run time", list, section)
	}
	return nil
}

// appendDefaults adds a top-level block, indented one level, at the end of
// doc's defaults: section (created when missing).
func appendDefaults(doc *config.Document, block string) {
	lines := strings.Split(strings.TrimRight(block, "\n"), "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			lines[i] = "  " + l
		}
	}
	head := workflowconfig.DefaultsKey + ":"
	if existing, found := doc.Block(workflowconfig.DefaultsKey); found {
		head = strings.TrimRight(existing, "\n")
	}
	doc.SetBlock(workflowconfig.DefaultsKey, head+"\n"+strings.Join(lines, "\n"))
}

// seedTaskProvider copies the user's tasks.provider (userProvider) into a
// project file that has none: the user's no longer applies at run time,
// so a repository that relied on it keeps its provider.
func seedTaskProvider(p *Plan, o Options, doc *config.Document) {
	if _, set := doc.Scalar("tasks.provider"); set {
		return
	}
	v := userProvider(o)
	if v == "" || slices.Contains(RemovedProviders, v) {
		return
	}
	doc.SetScalar("tasks.provider", v)
	p.note("tasks.provider: %s copied from the user config into %s: the user's no longer applies at run time", v, paths.ProjectConfigFile)
}
