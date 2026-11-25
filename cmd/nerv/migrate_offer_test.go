package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
)

// releaseUserConfig is a user config.yaml whose personal deploy key and
// defaults.deploy seed belong in nerv-release's global release.yaml.
const releaseUserConfig = "deploy:\n  providers:\n    ssh-compose:\n      ssh_auth: key\ndefaults:\n  deploy:\n    host: github\n"

// taskUserConfig is a user config.yaml whose personal tasks: key belongs
// in nerv-task's global task.yaml.
const taskUserConfig = "tasks:\n  timer_store: ~/t.json\n"

// withCatalogueTools fakes the named catalogue tools as found on PATH,
// keeping every other LookPath answer of opts (S3).
func withCatalogueTools(opts options, tools ...string) options {
	previous := opts.LookPath
	opts.GOOS = "linux"
	opts.LookPath = func(name string) (string, error) {
		if slices.Contains(tools, name) {
			return "/usr/bin/" + name, nil
		}
		return previous(name)
	}
	return opts
}

// migrateOfferHome is a fake home with NERV installed and a user config
// holding content; it returns the home and the user config's path.
func migrateOfferHome(t *testing.T, content string) (string, string) {
	t.Helper()
	home := t.TempDir()
	seedInstalledPluginsForCLI(t, home)
	user := paths.Resolve(home).UserConfig
	if content != "" {
		writeFile(t, user, content)
	}
	return home, user
}

func runInstallCLI(t *testing.T, opts options, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(append([]string{"install"}, args...), &out, &errOut, opts)
	return code, out.String() + errOut.String()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// On a terminal, with nerv-release installed and its sections still in
// the user config.yaml, nerv install shows the steps nerv migrate --user
// would take and runs them on "y": the global release.yaml is written and
// the sections leave config.yaml.
func TestInstall_OffersMigrateWhenAToolsSectionsArePending_Yes(t *testing.T) {
	home, user := migrateOfferHome(t, releaseUserConfig)
	opts := withCatalogueTools(testOptions(home), "nerv-release")
	opts.Runner = baseInstallRunner()
	opts.IsTerminal = func() bool { return true }
	// "y" accepts the migration.
	opts.Stdin = strings.NewReader("y\n")

	code, out := runInstallCLI(t, opts, "--no-tools", "--no-skills", "--no-configure")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	contains(t, "the output", out,
		"=== Move settings to the NERV-ecosystem tools ===\n",
		"nerv-release reads its settings from its own file, but your user config.yaml still has them. nerv migrate --user would take these steps:\n",
		"release.yaml",
		"Run nerv migrate --user now? [y/N]: ",
		"nerv migrate: ")
	global := filepath.Join(filepath.Dir(user), "release.yaml")
	if got, want := readFile(t, global), releaseUserConfig; got != want {
		t.Errorf("release.yaml:\n%s\nwant:\n%s", got, want)
	}
	if rest := readFile(t, user); strings.Contains(rest, "ssh_auth") || strings.Contains(rest, "host: github") {
		t.Errorf("the moved keys are still in config.yaml:\n%s", rest)
	}
}

// On "n" (or no answer) nothing is written, and the command to run later
// is named.
func TestInstall_OffersMigrateWhenAToolsSectionsArePending_No(t *testing.T) {
	home, user := migrateOfferHome(t, taskUserConfig)
	opts := withCatalogueTools(testOptions(home), "nerv-task")
	opts.Runner = baseInstallRunner()
	opts.IsTerminal = func() bool { return true }
	opts.Stdin = strings.NewReader("n\n")

	code, out := runInstallCLI(t, opts, "--no-tools", "--no-skills", "--no-configure")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	contains(t, "the output", out,
		"nerv-task reads its settings from its own file",
		"Run nerv migrate --user now? [y/N]: ",
		"Not migrated: run `nerv migrate --user` when you are ready.\n")
	if got := readFile(t, user); got != taskUserConfig {
		t.Errorf("config.yaml changed:\n%s", got)
	}
	if fileExists(filepath.Join(filepath.Dir(user), "task.yaml")) {
		t.Error("task.yaml was written without consent")
	}
}

// Without a terminal, nerv install only names the command: nothing is
// written.
func TestInstall_MigrateOfferNotATerminalPrintsTheHint(t *testing.T) {
	home, user := migrateOfferHome(t, releaseUserConfig)
	opts := withCatalogueTools(testOptions(home), "nerv-release")
	opts.Runner = baseInstallRunner()

	code, out := runInstallCLI(t, opts, "--no-skills", "--no-configure")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	contains(t, "the output", out,
		"\nnerv-release reads its settings from its own file, but your user config.yaml still has them: run `nerv migrate --user --dry-run` to see the steps, then `nerv migrate --user`.\n")
	lacks(t, "the output", out, "[y/N]: ")
	if got := readFile(t, user); got != releaseUserConfig {
		t.Errorf("config.yaml changed:\n%s", got)
	}
	if fileExists(filepath.Join(filepath.Dir(user), "release.yaml")) {
		t.Error("release.yaml was written without a terminal")
	}
}

// No offer when no tool is installed (the sections keep meaning nerv's
// own extensions) or when nothing is pending: the output and the stdin
// script are the same as before (regression, L3).
func TestInstall_NoMigrateOfferWhenNothingIsPending(t *testing.T) {
	tests := []struct {
		name, config string
		tools        []string
	}{
		{"no tool installed", releaseUserConfig, nil},
		{"tool installed, nothing to move", "models:\n  shinji: sonnet\n", []string{"nerv-release", "nerv-task"}},
		{"tool installed, no user config", "", []string{"nerv-release"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, user := migrateOfferHome(t, tt.config)
			opts := withCatalogueTools(testOptions(home), tt.tools...)
			opts.Runner = baseInstallRunner()
			opts.IsTerminal = func() bool { return true }
			opts.Stdin = strings.NewReader("n\nn\n")

			code, out := runInstallCLI(t, opts, "--no-tools", "--no-skills", "--no-configure")
			if code != 0 {
				t.Fatalf("exit %d\n%s", code, out)
			}
			lacks(t, "the output", out, "nerv migrate", "NERV-ecosystem tools ===")
			if tt.config != "" {
				if got := readFile(t, user); got != tt.config {
					t.Errorf("config.yaml changed:\n%s", got)
				}
			}
		})
	}
}

// nerv upgrade offers the same migration after its ecosystem-tools step.
func TestUpgrade_OffersMigrateWhenAToolsSectionsArePending(t *testing.T) {
	t.Run("yes migrates", func(t *testing.T) {
		w := newUpgradeWorld(t, "v5.1.0", defaultReleases...)
		user := paths.Resolve(w.opts.Home).UserConfig
		writeFile(t, user, releaseUserConfig)
		w.opts = withCatalogueTools(w.opts, "nerv-release")
		w.isTerminal = true
		w.opts.Stdin = strings.NewReader("y\n")

		code, out, errOut := w.upgrade("--no-tools")
		if code != 0 {
			t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
		}
		contains(t, "the output", out, "Run nerv migrate --user now? [y/N]: ")
		if got := readFile(t, filepath.Join(filepath.Dir(user), "release.yaml")); got != releaseUserConfig {
			t.Errorf("release.yaml:\n%s", got)
		}
	})
	t.Run("no writes nothing", func(t *testing.T) {
		w := newUpgradeWorld(t, "v5.1.0", defaultReleases...)
		user := paths.Resolve(w.opts.Home).UserConfig
		writeFile(t, user, taskUserConfig)
		w.opts = withCatalogueTools(w.opts, "nerv-task")
		w.isTerminal = true
		w.opts.Stdin = strings.NewReader("n\n")

		code, out, errOut := w.upgrade("--no-tools")
		if code != 0 {
			t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
		}
		contains(t, "the output", out, "Not migrated: run `nerv migrate --user` when you are ready.\n")
		if got := readFile(t, user); got != taskUserConfig {
			t.Errorf("config.yaml changed:\n%s", got)
		}
	})
	t.Run("not a terminal prints the hint", func(t *testing.T) {
		w := newUpgradeWorld(t, "v5.1.0", defaultReleases...)
		user := paths.Resolve(w.opts.Home).UserConfig
		writeFile(t, user, releaseUserConfig)
		w.opts = withCatalogueTools(w.opts, "nerv-release")

		code, out, errOut := w.upgrade("--no-tools")
		if code != 0 {
			t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
		}
		contains(t, "the output", out, "run `nerv migrate --user --dry-run` to see the steps, then `nerv migrate --user`.\n")
		if got := readFile(t, user); got != releaseUserConfig {
			t.Errorf("config.yaml changed:\n%s", got)
		}
	})
	t.Run("nothing pending, no offer", func(t *testing.T) {
		w := newUpgradeWorld(t, "v5.1.0", defaultReleases...)
		writeFile(t, paths.Resolve(w.opts.Home).UserConfig, releaseUserConfig)
		w.isTerminal = true

		code, out, errOut := w.upgrade("--no-tools")
		if code != 0 {
			t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
		}
		lacks(t, "the output", out, "nerv migrate")
	})
}

// nerv upgrade's nested install never offers the migration itself: the
// upgrade offers it once, after its own ecosystem-tools step.
func TestInstall_SkipMigrateOfferSkipsTheMigrateOffer(t *testing.T) {
	home, user := migrateOfferHome(t, releaseUserConfig)
	opts := withCatalogueTools(testOptions(home), "nerv-release")
	opts.Runner = baseInstallRunner()

	code, out := runInstallCLI(t, opts, "--skip-ecosystem-tools", "--skip-migrate-offer", "--no-skills", "--no-configure")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	lacks(t, "the output", out, "nerv migrate")
	if got := readFile(t, user); got != releaseUserConfig {
		t.Errorf("config.yaml changed:\n%s", got)
	}
}

// The install an older nerv's upgrade launches passes only
// --skip-ecosystem-tools (6.5.1 knows no --skip-migrate-offer): the first
// upgrade into this version must still offer nerv migrate.
func TestInstall_SkipEcosystemToolsAloneStillOffersMigrate(t *testing.T) {
	home, user := migrateOfferHome(t, releaseUserConfig)
	opts := withCatalogueTools(testOptions(home), "nerv-release")
	opts.Runner = baseInstallRunner()

	code, out := runInstallCLI(t, opts, "--skip-gentle-ai-upgrade", "--skip-ecosystem-tools", "--no-skills", "--no-configure")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "nerv migrate --user") {
		t.Errorf("the install an older upgrade launches does not offer nerv migrate:\n%s", out)
	}
	if got := readFile(t, user); got != releaseUserConfig {
		t.Errorf("config.yaml changed without consent:\n%s", got)
	}
}

// nerv doctor sees a pending task.yaml or release.yaml move, in the user
// configuration and in the repository, and names the command to run.
func TestDoctor_SeesAPendingToolFileMove(t *testing.T) {
	t.Run("user release.yaml", func(t *testing.T) {
		w := newDoctorWorld(t)
		w.opts = withCatalogueTools(w.opts, "nerv-release")
		w.write(paths.Resolve(w.home).UserConfig, releaseUserConfig)
		before := treeHash(t, w.home)

		code, out, _ := w.doctor()
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		contains(t, "the report", out,
			"  [!!]  tool config  nerv-release is installed, but your user config.yaml still has its settings: nerv migrate --user moves them to "+filepath.Join(filepath.Dir(paths.Resolve(w.home).UserConfig), "release.yaml")+"\n",
			"Run nerv migrate --user --dry-run to see the steps, then nerv migrate --user")
		if treeHash(t, w.home) != before {
			t.Error("nerv doctor changed the home")
		}
	})
	t.Run("user task.yaml", func(t *testing.T) {
		w := newDoctorWorld(t)
		w.opts = withCatalogueTools(w.opts, "nerv-task")
		w.write(paths.Resolve(w.home).UserConfig, taskUserConfig)

		_, out, _ := w.doctor()
		contains(t, "the report", out,
			"  [!!]  tool config  nerv-task is installed, but your user config.yaml still has its settings: nerv migrate --user moves them to "+filepath.Join(filepath.Dir(paths.Resolve(w.home).UserConfig), "task.yaml")+"\n")
	})
	t.Run("repository release.yaml", func(t *testing.T) {
		w := newDoctorWorld(t)
		w.opts = withCatalogueTools(w.opts, "nerv-release")
		w.write(filepath.Join(w.top, ".ai-workflow", "config.yaml"), "enabled: true\nrelease:\n  projects:\n    - name: api\n      path: .\n      build: bash\n")
		before := treeHash(t, w.top)

		_, out, _ := w.doctor()
		contains(t, "the report", out,
			"  [!!]  migration    nerv migrate has 3 step(s) to take, including moving settings to .ai-workflow/release.yaml (nerv-release is installed)\n",
			"Run nerv migrate --dry-run to see the steps, then nerv migrate")
		if treeHash(t, w.top) != before {
			t.Error("nerv doctor changed the repository")
		}
	})
	t.Run("repository legacy section", func(t *testing.T) {
		w := newDoctorWorld(t)
		w.opts = withCatalogueTools(w.opts, "nerv-release")
		w.write(filepath.Join(w.top, ".ai-workflow", "config.yaml"), "enabled: true\nship:\n  projects: []\n")

		_, out, _ := w.doctor()
		contains(t, "the report", out,
			"including moving settings to .ai-workflow/release.yaml (nerv-release is installed), then nerv-release migrate converts an older nerv's release and deploy configuration\n")
	})
	t.Run("repository legacy deploy provider only", func(t *testing.T) {
		w := newDoctorWorld(t)
		w.opts = withCatalogueTools(w.opts, "nerv-release")
		w.write(filepath.Join(w.top, ".ai-workflow", "config.yaml"), "enabled: true\n")
		w.write(filepath.Join(w.top, ".ai-workflow", "providers", "deploy-k8s", "provider.yaml"), "name: k8s\nextension: deploy\n")

		_, out, _ := w.doctor()
		contains(t, "the report", out,
			"  [!!]  migration    nerv migrate has 0 step(s) to take, then nerv-release migrate converts an older nerv's release and deploy configuration\n")
	})
	t.Run("repository task.yaml", func(t *testing.T) {
		w := newDoctorWorld(t)
		w.opts = withCatalogueTools(w.opts, "nerv-task")
		w.write(filepath.Join(w.top, ".ai-workflow", "config.yaml"), "enabled: true\ntasks:\n  provider: teamwork\n")

		_, out, _ := w.doctor()
		contains(t, "the report", out, "including moving settings to .ai-workflow/task.yaml (nerv-task is installed)\n")
	})
	t.Run("tools installed, nothing pending", func(t *testing.T) {
		w := newDoctorWorld(t)
		w.opts = withCatalogueTools(w.opts, "nerv-release", "nerv-task")
		w.write(filepath.Join(w.top, ".ai-workflow", "config.yaml"), "enabled: true\n")

		_, out, _ := w.doctor()
		contains(t, "the report", out, "  [ok]  migration    nothing to migrate\n")
		lacks(t, "the report", out, "tool config", "nerv migrate --user")
	})
}
