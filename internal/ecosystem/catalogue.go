// Package ecosystem is the catalogue of released NERV-ecosystem tools --
// standalone companion CLIs/plugins that replace the extensions nerv used
// to ship (nerv-task, replacing the task and task-teamwork extensions;
// nerv-release, replacing the release extension and its providers) --
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
	// Commands names the slash commands the tool brings, such as
	// "/task:*": a user without the tool goes without them.
	Commands string
	// Replaces are the plugins of the nerv marketplace this tool replaces,
	// which nerv no longer ships: nerv install and nerv upgrade disable
	// them (they are orphans, with no source left).
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
			Commands:    "/task:*",
			Replaces:    []string{"task", "task-teamwork"},
		},
		{
			Name:        "nerv-release",
			Repo:        "war-apps/nerv-release-by-gentle-ai",
			Binary:      "nerv-release",
			Description: "release and deploy (/release:*), replacing the release extension and its providers",
			Commands:    "/release:*",
			Replaces: []string{
				"release", "release-bash", "release-conventional-commits", "release-docker-compose",
				"release-github", "release-script", "release-ssh-compose",
				// The deploy extension and its providers, merged into
				// release in 5.2.0.
				"deploy", "deploy-bash", "deploy-github", "deploy-ssh-compose",
			},
		},
	}
}
