package migrate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/war-apps/nerv-by-gentle-ai/internal/paths"
	"gopkg.in/yaml.v3"
)

// The ODD layout (plugin/skills/nerv-orchestrator/references/odd-document.md).
const (
	oddTasksDir = "odd/tasks"
	locksDir    = "locks"
	// runsDir holds a DIRECT or LIGHT run under the runtime folder.
	runsDir = "runs"
)

// tddDefault is the TDD mode the old openspec/config.yaml set.
type tddDefault struct {
	// mode is strict or standard; runner the test command ("none" when
	// there is none).
	mode, runner string
}

// folded are the change files the ODD document absorbs.
var folded = []string{"proposal.md", "design.md", "tasks.md"}

// planODD converts every in-flight openspec change of root into an ODD
// feature document (its proposal, specs, design and tasks folded into
// Specs, Tasks and Log) and moves its run files to odd/<change>/nerv/ and
// its lock to .nerv/locks/<change>.lock. Archived changes and
// openspec/specs stay. It returns the TDD default of openspec/config.yaml
// (nil without one).
func planODD(p *Plan, o Options) (*tddDefault, error) {
	root := o.RepoRoot
	base := filepath.Join(root, "openspec")
	info, err := os.Stat(base)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.IsDir()) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	tdd, err := readTDD(p, base)
	if err != nil {
		return nil, err
	}
	changes := filepath.Join(base, "changes")
	entries, err := os.ReadDir(changes)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == "archive" {
			archived, _ := os.ReadDir(filepath.Join(changes, "archive"))
			p.note("openspec/changes/archive: %d archived change(s) stay where they are, read-only", len(archived))
			continue
		}
		if err := planChange(p, o, e.Name(), tdd); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(filepath.Join(base, "specs")); err == nil {
		p.note("openspec/specs stays where it is, read-only")
	}
	return tdd, nil
}

// readTDD reads strict_tdd and testing.runner of openspec/config.yaml.
func readTDD(p *Plan, base string) (*tddDefault, error) {
	data, err := os.ReadFile(filepath.Join(base, "config.yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg struct {
		StrictTDD *bool `yaml:"strict_tdd"`
		Testing   struct {
			Runner string `yaml:"runner"`
		} `yaml:"testing"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		p.warn("openspec/config.yaml is not valid YAML (%v): its strict_tdd is not carried over", err)
		return nil, nil
	}
	if cfg.StrictTDD == nil && cfg.Testing.Runner == "" {
		return nil, nil
	}
	t := &tddDefault{mode: "standard", runner: strings.TrimSpace(cfg.Testing.Runner)}
	if cfg.StrictTDD != nil && *cfg.StrictTDD {
		t.mode = "strict"
	}
	if t.runner == "" {
		t.runner = "none"
	}
	p.note("openspec/config.yaml stays (its context and rules are no longer read); strict_tdd and testing.runner become odd.tdd: %s and odd.test_runner: %s", t.mode, t.runner)
	return t, nil
}

// changeState is what the document reads of a change's state.yaml.
type changeState struct {
	DependsOn []string `yaml:"dependsOn"`
	Nerv      struct {
		Path    string `yaml:"path"`
		TaskRef string `yaml:"task_ref"`
		Branch  string `yaml:"branch"`
		Base    string `yaml:"base"`
		Created string `yaml:"created_at"`
	} `yaml:"nerv"`
}

// docMarker is the line nerv migrate writes under the title of the ODD
// document it generates, so a rerun after an interrupted run recognises
// its own output.
func docMarker(name string) string {
	return "<!-- nerv migrate: openspec/changes/" + name + " -->"
}

// staleAfter is the orchestrator lock's stale threshold
// (plugin/skills/nerv-orchestrator/SKILL.md, "Staleness").
const staleAfter = 15 * time.Minute

// orchestratorLock is what nerv migrate reads of a change's lock.
type orchestratorLock struct {
	SessionID   string `yaml:"session_id"`
	Host        string `yaml:"host"`
	HeartbeatAt string `yaml:"heartbeat_at"`
	WaitingOn   string `yaml:"waiting_on"`
}

// heldLock reports the lock in data when it is held on host at now: a
// heartbeat younger than staleAfter, or a run waiting on the user at any
// age (the orchestrator's rule). A lock of another host, or one that does
// not parse, is not held here.
func heldLock(data []byte, host string, now time.Time) (*orchestratorLock, bool) {
	var l orchestratorLock
	if yaml.Unmarshal(data, &l) != nil || l.Host == "" || !strings.EqualFold(l.Host, host) {
		return nil, false
	}
	if strings.EqualFold(l.WaitingOn, "user") {
		return &l, true
	}
	hb, err := time.Parse(time.RFC3339, l.HeartbeatAt)
	if err != nil || now.Sub(hb) >= staleAfter {
		return nil, false
	}
	return &l, true
}

// planChange converts one in-flight change: a FULL one into an ODD
// document plus odd/<change>/nerv/, a DIRECT or LIGHT one (L20, "Solo
// FULL") into .nerv/runs/<change>/ with no document.
func planChange(p *Plan, o Options, name string, tdd *tddDefault) error {
	root, now := o.RepoRoot, o.Now
	rel := "openspec/changes/" + name
	dir := filepath.Join(root, filepath.FromSlash(rel))
	read := func(f string) string {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return ""
		}
		return string(data)
	}
	var state changeState
	if s := read("state.yaml"); s != "" {
		if err := yaml.Unmarshal([]byte(s), &state); err != nil {
			p.warn("%s/state.yaml is not valid YAML (%v): the document's header leaves the branch unknown", rel, err)
		}
	}
	tier := strings.ToLower(strings.TrimSpace(state.Nerv.Path))
	full := tier != "direct" && tier != "light"
	if full && name == "tasks" {
		p.warn("%s: a feature named tasks would collide with odd/tasks/; rename the folder and run nerv migrate again (left where it is)", rel)
		return nil
	}
	doc := filepath.Join(root, filepath.FromSlash(oddTasksDir), name+".md")
	writeDoc := full
	if full {
		if existing, err := os.ReadFile(doc); err == nil && strings.Contains(string(existing), docMarker(name)) {
			writeDoc = false
			p.note("%s/%s.md was written by an interrupted nerv migrate: it is kept, and the run files are moved now", oddTasksDir, name)
		} else if _, err := os.Lstat(doc); err == nil {
			return refuse("%s cannot be converted: %s already exists (merge them by hand, then remove %s)", rel, filepath.ToSlash(filepath.Join(oddTasksDir, name+".md")), rel)
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, "nerv", ".orchestrator.lock")); err == nil {
		host := o.Host
		if host == "" {
			host, _ = os.Hostname()
		}
		if l, held := heldLock(data, host, now); held {
			if !o.Force {
				return refuse("%s is still running: session %s holds its lock on %s (heartbeat %s, waiting on %s); let that run end or stop it, or pass --force to move it anyway", rel, l.SessionID, l.Host, orDefault(l.HeartbeatAt, "none"), orDefault(l.WaitingOn, "none"))
			}
			p.warn("%s: --force moves the lock session %s still holds (heartbeat %s); that session loses it", rel, l.SessionID, orDefault(l.HeartbeatAt, "none"))
		}
	}
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symbolic link: move it by hand", path)
		}
		if !d.IsDir() {
			r, _ := filepath.Rel(dir, path)
			files = append(files, filepath.ToSlash(r))
		}
		return nil
	})
	if err != nil {
		return refuse("%s: %v", rel, err)
	}
	slices.Sort(files)

	runDir := filepath.Join(root, "odd", name, "nerv")
	runRel := "odd/" + name + "/nerv/"
	if !full {
		runDir = filepath.Join(paths.RuntimeDir(root), runsDir, name)
		runRel = ".nerv/" + runsDir + "/" + name + "/"
	}
	var specs []string
	sources := map[string]string{}
	for _, f := range files {
		src := filepath.Join(dir, filepath.FromSlash(f))
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		var dst string
		switch {
		case f == "nerv/.orchestrator.lock":
			dst = filepath.Join(paths.RuntimeDir(root), locksDir, name+".lock")
			p.warn("%s: its orchestrator lock moves to .nerv/locks/%s.lock: make sure no session is still running this change (a running session would lose its lock)", rel, name)
		case full && slices.Contains(folded, f):
		case full && strings.HasPrefix(f, "specs/"):
			specs = append(specs, f)
		case strings.HasPrefix(f, "nerv/"):
			dst = filepath.Join(runDir, filepath.FromSlash(strings.TrimPrefix(f, "nerv/")))
		default:
			dst = filepath.Join(runDir, filepath.FromSlash(f))
		}
		stop := filepath.Dir(dir)
		if dst != "" {
			if other, dup := sources[dst]; dup {
				return refuse("%s: %s and %s would both move to %s; rename one by hand", rel, other, f, dst)
			}
			sources[dst] = f
			existing, err := os.ReadFile(dst)
			switch {
			case err == nil && string(existing) != string(data):
				return refuse("%s/%s cannot move: %s already exists with other content", rel, f, dst)
			case err != nil:
				p.Changes = append(p.Changes, Change{Kind: Rename, Path: src, To: dst, Old: data, StopAt: stop})
				continue
			}
		}
		why := "an identical copy is already in place"
		if dst == "" {
			why = "its content is in " + oddTasksDir + "/" + name + ".md"
		}
		p.Changes = append(p.Changes, Change{Kind: Remove, Path: src, Old: data, StopAt: stop, Why: why})
	}
	if !full {
		p.note("%s is a %s run: it moved to %s and its lock to .nerv/locks/%s.lock, with no ODD document (only FULL runs get one); .nerv/ is git-ignored, so its files leave version control", rel, strings.ToUpper(tier), runRel, name)
		return nil
	}
	if writeDoc {
		text := oddDocument(name, state, tdd, read, specs, now)
		p.Changes = append(p.Changes, Change{Kind: Write, Path: doc, New: []byte(text), Why: "the task document of " + rel})
	}
	return nil
}

// headingRe matches a Markdown ATX heading.
var headingRe = regexp.MustCompile(`^(#{1,6})(\s)`)

// demote lowers every heading of a Markdown text (outside code fences)
// by levels, at most to level 6.
func demote(text string, levels int) string {
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n"), "\n")
	fence := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		if m := headingRe.FindStringSubmatch(l); m != nil {
			n := min(len(m[1])+levels, 6)
			lines[i] = strings.Repeat("#", n) + l[len(m[1]):]
		}
	}
	return strings.Join(lines, "\n")
}

var (
	taskHeadingRe = regexp.MustCompile(`^##\s+(T\d+)[.:]?\s*(.*)$`)
	taskFieldRe   = regexp.MustCompile(`^-\s+(id|pilot|depends_on|status|specs):\s*(.*)$`)
)

// taskRow is one task of an openspec tasks.md.
type taskRow struct {
	id, title, specs, pilot, deps, status string
}

func parseTasks(text string) []taskRow {
	var rows []taskRow
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if m := taskHeadingRe.FindStringSubmatch(l); m != nil {
			rows = append(rows, taskRow{id: m[1], title: strings.TrimSpace(m[2])})
			continue
		}
		if len(rows) == 0 {
			continue
		}
		if m := taskFieldRe.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			r := &rows[len(rows)-1]
			v := strings.TrimSpace(m[2])
			switch m[1] {
			case "pilot":
				r.pilot = v
			case "depends_on":
				r.deps = strings.Trim(v, "[]")
			case "status":
				r.status = v
			case "specs":
				r.specs = v
			}
		}
	}
	return rows
}

// oddStatus maps an openspec task status to the ODD document's.
func oddStatus(s string) string {
	switch strings.ToLower(s) {
	case "", "planned", "pending", "todo":
		return "pending"
	case "done", "completed", "complete":
		return "done"
	case "in_progress", "in-progress", "in progress", "implementing":
		return "in progress"
	}
	return s
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return strings.ReplaceAll(s, "|", `\|`)
}

// oddDocument builds the ODD feature document of a change.
func oddDocument(name string, state changeState, tdd *tddDefault, read func(string) string, specs []string, now time.Time) string {
	var b strings.Builder
	date := now.Format("2006-01-02")
	fmt.Fprintf(&b, "# %s\n\n%s\n\n", name, docMarker(name))
	branch := orDefault(state.Nerv.Branch, "unknown")
	fmt.Fprintf(&b, "Migrated from `openspec/changes/%s/` by `nerv migrate` on %s; run folder `odd/%s/nerv/`. Branch: `%s`", name, date, name, branch)
	if state.Nerv.TaskRef != "" {
		fmt.Fprintf(&b, ", task %s", state.Nerv.TaskRef)
	}
	if state.Nerv.Path != "" {
		fmt.Fprintf(&b, ", tier %s", strings.ToUpper(state.Nerv.Path))
	}
	b.WriteString(".\n")
	switch {
	case tdd == nil:
		b.WriteString("Test runner: none recorded (standard TDD).\n")
	case tdd.mode == "strict":
		fmt.Fprintf(&b, "Test runner: %s (strict TDD).\n", tdd.runner)
	default:
		fmt.Fprintf(&b, "Test runner: %s (standard TDD).\n", tdd.runner)
	}

	b.WriteString("\n## Specs\n\n")
	wrote := false
	for _, f := range specs {
		if text := read(f); strings.TrimSpace(text) != "" {
			fmt.Fprintf(&b, "_From `%s`:_\n\n%s\n\n", f, demote(text, 2))
			wrote = true
		}
	}
	if text := read("proposal.md"); strings.TrimSpace(text) != "" {
		fmt.Fprintf(&b, "_From `proposal.md` (scope and acceptance criteria):_\n\n%s\n\n", demote(text, 2))
		wrote = true
	}
	if !wrote {
		b.WriteString("No specs were recorded before the migration.\n\n")
	}

	b.WriteString("## Tasks\n\n")
	tasks := read("tasks.md")
	rows := parseTasks(tasks)
	route := "—"
	switch strings.ToLower(state.Nerv.Path) {
	case "direct":
		route = "inline (DIRECT)"
	case "light":
		route = "delegated (LIGHT)"
	case "full":
		route = "delegated (FULL)"
	}
	if len(rows) > 0 {
		b.WriteString("| ID | Specs | Route | Pilot | Depends on | Criticality | MAGI | Status | Commit |\n|---|---|---|---|---|---|---|---|---|\n")
		for _, r := range rows {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | — | — | %s | — |\n", r.id, dash(r.specs), route, dash(r.pilot), dash(r.deps), oddStatus(r.status))
		}
		b.WriteString("\n")
		for _, r := range rows {
			fmt.Fprintf(&b, "- **%s:** %s\n", r.id, r.title)
		}
		b.WriteString("\n")
	}
	if strings.TrimSpace(tasks) != "" {
		fmt.Fprintf(&b, "_From `tasks.md`:_\n\n%s\n\n", demote(tasks, 2))
	} else {
		b.WriteString("No tasks were recorded before the migration.\n\n")
	}

	b.WriteString("## Log\n\n")
	n := 1
	entry := func(format string, args ...any) {
		fmt.Fprintf(&b, "- **L%d** "+format+"\n", append([]any{n}, args...)...)
		n++
	}
	created := orDefault(state.Nerv.Created, "unknown date")
	entry("(%s, migrated): the original request was not recorded on its own before the migration; the proposal under `## Specs` quotes it.", created)
	if design := read("design.md"); strings.TrimSpace(design) != "" {
		entry("design (from `design.md`):")
		fmt.Fprintf(&b, "\n%s\n\n", demote(design, 2))
	}
	for _, f := range []string{"exploration.md", "exploration-light.md"} {
		if read(f) != "" {
			entry("exploration: `nerv/%s`.", f)
		}
	}
	if read("nerv/test-plan.md") != "" {
		entry("test plan: `nerv/test-plan.md`.")
	}
	entry("(%s, nerv migrate): the change's run files moved to `odd/%s/nerv/` and its lock to `.nerv/locks/%s.lock`. Next step: resume the run from `nerv/state.yaml`.", date, name, name)
	return b.String()
}
