// Package ecosystem is the catalogue of released NERV-ecosystem tools --
// standalone companion CLIs/plugins that replace one or more of nerv's own
// extensions (nerv-task, replacing the task and task-teamwork extensions;
// nerv-release, replacing the release extension and its six providers) --
// plus the detection, release lookup and execution glue "nerv install" and
// "nerv upgrade" need to offer them. Every tool implements the S10
// contract: "<tool> version [--json]", install, upgrade, configure, init,
// and ships its official install scripts as scripts/install.sh and
// scripts/install.ps1 on its main branch; nerv drives a tool only through
// these.
package ecosystem

// Tool is one released ecosystem tool nerv can offer to install or
// upgrade.
type Tool struct {
	// Name is the tool's own name, equal to Binary for every tool today.
	Name string
	// Repo is the tool's GitHub repository, "<owner>/<repo>".
	Repo string
	// Binary is the executable name LookPath, the default install
	// directory and the install scripts resolve.
	Binary string
	// Description is a one-line summary, shown next to its state.
	Description string
	// Replaces are the nerv extension names this tool makes redundant:
	// nerv must never offer, select or re-enable them once the tool is
	// installed, or selected in this run.
	Replaces []string
}

// Catalogue is every ecosystem tool nerv knows about, in offer order.
func Catalogue() []Tool {
	return []Tool{
		{
			Name:        "nerv-task",
			Repo:        "war-apps/nerv-task-by-gentle-ai",
			Binary:      "nerv-task",
			Description: "task tracking and time logging (/task:*), replacing the task and task-teamwork extensions",
			Replaces:    []string{"task", "task-teamwork"},
		},
		{
			Name:        "nerv-release",
			Repo:        "war-apps/nerv-release-by-gentle-ai",
			Binary:      "nerv-release",
			Description: "release and deploy (/release:*), replacing the release extension and its providers",
			Replaces: []string{
				"release", "release-bash", "release-conventional-commits", "release-docker-compose",
				"release-github", "release-script", "release-ssh-compose",
			},
		},
	}
}

// ReplacedExtensions is the deduplicated union, first-seen order, of
// every tool's Replaces whose Name is set in installed (a set of tool
// names considered installed, or selected in this run): the nerv
// extension names install/upgrade/configure must never again offer,
// select or re-enable.
func ReplacedExtensions(tools []Tool, installed map[string]bool) []string {
	var out []string
	seen := make(map[string]bool)
	for _, t := range tools {
		if !installed[t.Name] {
			continue
		}
		for _, name := range t.Replaces {
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// ReplacingTool finds the tool in tools whose Replaces names extension, so
// a refusal or a skip message can name it ("nerv-task already replaces
// task-teamwork").
func ReplacingTool(tools []Tool, extension string) (Tool, bool) {
	for _, t := range tools {
		for _, name := range t.Replaces {
			if name == extension {
				return t, true
			}
		}
	}
	return Tool{}, false
}
