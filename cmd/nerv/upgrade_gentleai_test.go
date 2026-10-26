package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/env/envtest"
)

// withGentleAI makes "gentle-ai --version" print version ("" leaves
// gentle-ai off PATH) and serves latest as gentle-ai's newest release.
func (w *upgradeWorld) withGentleAI(version, latest string) {
	runner := &envtest.FakeRunner{}
	if version != "" {
		runner.Responses = map[string]envtest.Response{"gentle-ai --version": {Stdout: "gentle-ai " + version + "\n"}}
	}
	w.opts.Runner = runner
	w.gentleLatest = latest
}

// gentleCalls are the gentle-ai runs, as their argument lines.
func (w *upgradeWorld) gentleCalls() []string {
	var calls []string
	for _, c := range w.installs {
		if c.Bin == "gentle-ai" {
			calls = append(calls, strings.Join(c.Args, " "))
		}
	}
	return calls
}

// nervInstalls are the runs of a nerv binary's install.
func (w *upgradeWorld) nervInstalls() []installCall {
	var calls []installCall
	for _, c := range w.installs {
		if c.Bin != "gentle-ai" {
			calls = append(calls, c)
		}
	}
	return calls
}

var gentleUpgradeCalls = []string{"upgrade gentle-ai", "sync"}

// A newer gentle-ai release of the same major version (4.x) is installed
// with gentle-ai upgrade and synced, and nerv's install runs again, even
// when nerv itself is up to date.
func TestUpgrade_GentleAISameMajorIsUpgraded(t *testing.T) {
	w := newUpgradeWorld(t, "v5.1.0", defaultReleases...)
	w.withGentleAI("4.2.0", "v4.3.0")
	code, out, errOut := w.upgrade()
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
	}
	if got := w.gentleCalls(); !reflect.DeepEqual(got, gentleUpgradeCalls) {
		t.Errorf("gentle-ai runs = %v, want %v", got, gentleUpgradeCalls)
	}
	if got := w.nervInstalls(); len(got) != 1 || got[0].Bin != w.exe || !reflect.DeepEqual(got[0].Args, []string{"install", "--no-configure"}) {
		t.Errorf("nerv installs = %+v, want one %s install --no-configure", got, w.exe)
	}
	if w.binary() != oldBinary {
		t.Errorf("nerv binary changed although it is up to date")
	}
	for _, line := range []string{
		"gentle-ai: installed 4.2.0, newest release v4.3.0\n",
		"Upgrading gentle-ai to v4.3.0: gentle-ai upgrade gentle-ai\n",
		"Running: gentle-ai sync\n",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("stdout lacks %q:\n%s", line, out)
		}
	}
}

// When nerv is replaced too, gentle-ai is upgraded before the new binary's
// install runs, so that install sees the new gentle-ai.
func TestUpgrade_GentleAIUpgradedBeforeNewInstall(t *testing.T) {
	w := newUpgradeWorld(t, "v5.0.0", defaultReleases...)
	w.withGentleAI("4.2.0", "v4.3.0")
	code, out, errOut := w.upgrade()
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
	}
	want := []installCall{{Bin: "gentle-ai", Args: []string{"upgrade", "gentle-ai"}}, {Bin: "gentle-ai", Args: []string{"sync"}}, {Bin: w.exe, Args: []string{"install", "--no-configure"}}}
	if !reflect.DeepEqual(w.installs, want) {
		t.Errorf("runs = %+v, want %+v", w.installs, want)
	}
	if got := w.binary(); got != newBinaryFor("v5.1.0") {
		t.Errorf("binary = %q, want v5.1.0", got)
	}
}

// A gentle-ai release of another major version is never installed: NERV
// supports 4.x only. gentle-ai already up to date is left alone too.
func TestUpgrade_GentleAINotUpgraded(t *testing.T) {
	tests := []struct{ name, installed, latest, line string }{
		{"new major", "4.2.0", "v5.0.0", "gentle-ai v5.0.0 is a new major version: NERV v5.1.0 supports gentle-ai 4.x, so gentle-ai stays on 4.2.0.\n"},
		{"up to date", "4.2.0", "v4.2.0", "gentle-ai 4.2.0 is up to date.\n"},
		{"newer than latest", "4.3.0", "v4.2.0", "gentle-ai 4.3.0 is up to date.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newUpgradeWorld(t, "v5.1.0", defaultReleases...)
			w.withGentleAI(tt.installed, tt.latest)
			code, out, errOut := w.upgrade()
			if code != 0 || errOut != "" {
				t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
			}
			if !strings.Contains(out, tt.line) {
				t.Errorf("stdout lacks %q:\n%s", tt.line, out)
			}
			if got := w.gentleCalls(); len(got) != 0 {
				t.Errorf("gentle-ai runs = %v, want none", got)
			}
			w.assertUntouched()
		})
	}
}

// Without gentle-ai on PATH, or with a gentle-ai that is not 4.x, nerv
// upgrade does not ask for gentle-ai's releases and runs no gentle-ai.
func TestUpgrade_GentleAISkipped(t *testing.T) {
	tests := []struct{ name, installed, line string }{
		{"not on PATH", "", "gentle-ai is not on PATH; not upgrading it. NERV needs gentle-ai 4.x: https://github.com/Gentleman-Programming/gentle-ai\n"},
		{"other major", "3.9.0", "gentle-ai 3.9.0 is not 4.x, the major version NERV supports; not upgrading it. Install gentle-ai 4.x: https://github.com/Gentleman-Programming/gentle-ai\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newUpgradeWorld(t, "v5.1.0", defaultReleases...)
			w.withGentleAI(tt.installed, "v4.3.0")
			code, out, errOut := w.upgrade()
			if code != 0 || errOut != "" {
				t.Fatalf("exit %d, stderr %q\n%s", code, errOut, out)
			}
			if !strings.Contains(out, tt.line) {
				t.Errorf("stdout lacks %q:\n%s", tt.line, out)
			}
			if w.gentleRequests != 0 {
				t.Errorf("asked gentle-ai's releases %d time(s), want none", w.gentleRequests)
			}
			w.assertUntouched()
		})
	}
}

// A failed gentle-ai upgrade or release lookup is reported on stderr and
// exits 1, but the nerv upgrade still finishes: the new binary's install
// runs. A failed gentle-ai upgrade runs no sync.
func TestUpgrade_GentleAIFailure(t *testing.T) {
	t.Run("upgrade fails", func(t *testing.T) {
		w := newUpgradeWorld(t, "v5.0.0", defaultReleases...)
		w.withGentleAI("4.2.0", "v4.3.0")
		w.gentleRC = map[string]int{"upgrade gentle-ai": 3}
		code, out, errOut := w.upgrade()
		if code != 1 {
			t.Fatalf("exit %d, want 1\n%s", code, out)
		}
		if want := "nerv upgrade: gentle-ai: gentle-ai upgrade gentle-ai exited 3\n"; errOut != want {
			t.Errorf("stderr = %q, want %q", errOut, want)
		}
		if got := w.gentleCalls(); !reflect.DeepEqual(got, []string{"upgrade gentle-ai"}) {
			t.Errorf("gentle-ai runs = %v, want only the upgrade", got)
		}
		if got := w.nervInstalls(); len(got) != 1 {
			t.Errorf("nerv installs = %+v, want the new binary's install", got)
		}
	})
	t.Run("sync fails", func(t *testing.T) {
		w := newUpgradeWorld(t, "v5.1.0", defaultReleases...)
		w.withGentleAI("4.2.0", "v4.3.0")
		w.gentleRC = map[string]int{"sync": 2}
		code, out, errOut := w.upgrade()
		if code != 1 {
			t.Fatalf("exit %d, want 1\n%s", code, out)
		}
		if want := "nerv upgrade: gentle-ai: gentle-ai sync exited 2\n"; errOut != want {
			t.Errorf("stderr = %q, want %q", errOut, want)
		}
		if got := w.nervInstalls(); len(got) != 1 || got[0].Bin != w.exe {
			t.Errorf("nerv installs = %+v, want the running binary's install", got)
		}
	})
	t.Run("release lookup fails", func(t *testing.T) {
		w := newUpgradeWorld(t, "v5.1.0", defaultReleases...)
		w.withGentleAI("4.2.0", "")
		code, out, errOut := w.upgrade()
		if code != 1 {
			t.Fatalf("exit %d, want 1\n%s", code, out)
		}
		if !strings.HasPrefix(errOut, "nerv upgrade: gentle-ai: ") {
			t.Errorf("stderr = %q, want the gentle-ai error", errOut)
		}
		w.assertUntouched()
	})
}
