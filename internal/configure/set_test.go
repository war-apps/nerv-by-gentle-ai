package configure_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/war-apps/nerv-gentle-ai/internal/config"
	"github.com/war-apps/nerv-gentle-ai/internal/configure"
)

func newTestDeps(home string, now time.Time) configure.Deps {
	return configure.Deps{
		Home: home,
		Now:  func() time.Time { return now },
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// set-happy-path: one key changes, file differs in exactly that line, a
// backup is created.
// ---------------------------------------------------------------------------

func TestSet_HappyPath_ChangedOneLineDiffBackupCreated(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC))
	paths := configure.Paths{Config: configPath}

	result, err := configure.Set(deps, paths, []string{"git.base_branch=develop2"})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}
	if len(result.Changes) != 1 || result.Changes[0] != (configure.Change{Key: "git.base_branch", From: "develop", To: "develop2"}) {
		t.Errorf("Changes = %+v, want one git.base_branch develop->develop2", result.Changes)
	}
	if len(result.Written) != 1 || result.Written[0] != configPath {
		t.Errorf("Written = %v, want [%s]", result.Written, configPath)
	}
	if result.Backup == nil || *result.Backup == "" {
		t.Error("Backup = nil, want a backup path")
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if diff := diffLineCount(richFixtureLF, string(after)); diff != 1 {
		t.Errorf("diff line count = %d, want 1 (after=%q)", diff, after)
	}
	if !strings.Contains(string(after), "base_branch: develop2") {
		t.Error("new value not present")
	}
}

// ---------------------------------------------------------------------------
// group G regression: unknown content (known_projects, sources,
// sources_howto) survives a --set run byte for byte.
// ---------------------------------------------------------------------------

func TestSet_PreservesUnknownContentByteIdentical(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	if _, err := configure.Set(deps, paths, []string{"git.base_branch=develop2"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"known_projects:",
		"note: \"primary project for this team\"",
		"note: \"read-only archive, do not assign\"",
		"sources:",
		"sheet_id: abc123",
		"sources_howto: |",
		"How to add a new source:",
		"3. Fill in the fields.",
	} {
		if !strings.Contains(string(after), want) {
			t.Errorf("output missing unknown content %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// set-noop: same value as already on disk -> byte-identical, no backup,
// changed=false.
// ---------------------------------------------------------------------------

func TestSet_Noop_ByteIdenticalNoBackup(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	result, err := configure.Set(deps, paths, []string{"git.base_branch=develop"})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if result.Changed {
		t.Error("Changed = true, want false")
	}
	if result.Backup != nil {
		t.Errorf("Backup = %v, want nil", *result.Backup)
	}
	if len(result.Written) != 0 {
		t.Errorf("Written = %v, want empty", result.Written)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("dir has %d entries, want 1 (no backup file)", len(entries))
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != richFixtureLF {
		t.Error("file changed on a no-op --set")
	}
}

// ---------------------------------------------------------------------------
// set-unknown-key: refused, nothing written.
// ---------------------------------------------------------------------------

func TestSet_UnknownKey_RefusedNothingWritten(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	_, err := configure.Set(deps, paths, []string{"bogus.nonexistent.key=x"})
	if err == nil {
		t.Fatal("Set() error = nil, want a refusal")
	}
	var refusal *configure.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want *RefusalError", err)
	}
	var unknownKey *config.ErrUnknownKey
	if !errors.As(err, &unknownKey) {
		t.Fatalf("error = %v, want to wrap *config.ErrUnknownKey", err)
	}
	if !strings.Contains(err.Error(), "bogus.nonexistent.key") {
		t.Errorf("error message %q does not name the key", err.Error())
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != richFixtureLF {
		t.Error("file changed after a refused --set")
	}
}

// ---------------------------------------------------------------------------
// set-invalid-value: refused, message names the key.
// ---------------------------------------------------------------------------

func TestSet_InvalidEnumeratedValue_Refused(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	_, err := configure.Set(deps, paths, []string{"git.worktree=bogus"})
	if err == nil {
		t.Fatal("Set() error = nil, want a refusal")
	}
	var refusal *configure.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want *RefusalError", err)
	}
	if !strings.Contains(err.Error(), "git.worktree") {
		t.Errorf("error message %q does not name the key", err.Error())
	}
}

// ---------------------------------------------------------------------------
// set-malformed-entry: an entry without "=" is refused.
// ---------------------------------------------------------------------------

func TestSet_MalformedEntry_Refused(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	_, err := configure.Set(deps, paths, []string{"no-equals-sign"})
	if err == nil {
		t.Fatal("Set() error = nil, want a refusal")
	}
	var refusal *configure.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want *RefusalError", err)
	}
	if !strings.Contains(err.Error(), "no-equals-sign") {
		t.Errorf("error message %q does not name the entry", err.Error())
	}
}

// ---------------------------------------------------------------------------
// group K: set-batch-two-keys-one-write — two --set keys in one call ->
// changed true, 2 changes, 1 written path, 1 backup.
// ---------------------------------------------------------------------------

func TestSet_Batch_TwoKeysOneWriteOneBackup(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	result, err := configure.Set(deps, paths, []string{
		"git.branch_pattern={branchType}/{prefix}-{id}-{slug}",
		"skills.testing=tdd, playwright-best-practices, extra",
	})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}
	if len(result.Changes) != 2 {
		t.Fatalf("len(Changes) = %d, want 2 (%+v)", len(result.Changes), result.Changes)
	}
	if len(result.Written) != 1 {
		t.Errorf("len(Written) = %d, want 1", len(result.Written))
	}
	if result.Backup == nil {
		t.Error("Backup = nil, want one backup")
	}

	backups, _ := filepath.Glob(filepath.Join(dir, "nerv.yaml.bak-configure-*"))
	if len(backups) != 1 {
		t.Errorf("backup file count = %d, want 1", len(backups))
	}
}

// ---------------------------------------------------------------------------
// group K: set-batch-unknown-key-writes-nothing — a valid key plus an
// unknown key in the same batch -> refused, config byte-identical, no new
// backup.
// ---------------------------------------------------------------------------

func TestSet_Batch_UnknownKeyWritesNothing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, richFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	_, err := configure.Set(deps, paths, []string{"git.base_branch=zzz", "bogus.key=1"})
	if err == nil {
		t.Fatal("Set() error = nil, want a refusal")
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != richFixtureLF {
		t.Error("file changed after a batch containing an unknown key")
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "nerv.yaml.bak-configure-*"))
	if len(backups) != 0 {
		t.Errorf("backup file count = %d, want 0", len(backups))
	}
}

// ---------------------------------------------------------------------------
// group H (h2 ported): inline-form skills: — an empty --set batch (nothing
// requested) is a pure no-op against the inline fixture: byte-identical, no
// backup, and (since nothing was ever rewritten) still exactly one
// "skills:" key.
// ---------------------------------------------------------------------------

func TestSet_InlineFixture_EmptyBatchByteIdenticalNoBackup(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, inlineFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	result, err := configure.Set(deps, paths, []string{})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if result.Changed {
		t.Error("Changed = true, want false")
	}
	if result.Backup != nil {
		t.Errorf("Backup = %v, want nil", *result.Backup)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != inlineFixtureLF {
		t.Error("file changed on an empty --set batch")
	}
	if strings.Count(string(after), "skills:") != 1 {
		t.Errorf("skills: key count = %d, want 1", strings.Count(string(after), "skills:"))
	}

	backups, _ := filepath.Glob(filepath.Join(dir, "nerv.yaml.bak-configure-*"))
	if len(backups) != 0 {
		t.Errorf("backup file count = %d, want 0", len(backups))
	}
}

// ---------------------------------------------------------------------------
// group H (h3 ported): inline-form skills: — changing only skills.code
// leaves exactly one line different and every other category untouched.
// ---------------------------------------------------------------------------

func TestSet_InlineFixture_ChangeOneSkillsCategory(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, inlineFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	_, err := configure.Set(deps, paths, []string{"skills.code=dotnet-best-practices, xunit-testing"})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if diff := diffLineCount(inlineFixtureLF, string(after)); diff != 1 {
		t.Errorf("diff line count = %d, want 1 (after=%q)", diff, after)
	}
	if strings.Count(string(after), "skills:") != 1 {
		t.Errorf("skills: key count = %d, want 1 (no duplicate block)", strings.Count(string(after), "skills:"))
	}
	if !strings.Contains(string(after), "xunit-testing") {
		t.Error("new value not present")
	}
	if !strings.Contains(string(after), "testing: [tdd, playwright-best-practices]") ||
		!strings.Contains(string(after), "audit: [security-review, clean-code-guard]") {
		t.Error("other skills categories were not preserved")
	}
}

// ---------------------------------------------------------------------------
// group H (h4 ported): inline fixture with no artifacts: block — setting
// artifacts.commit appends the block exactly once, everything else
// preserved.
// ---------------------------------------------------------------------------

func TestSet_InlineFixture_AppendsArtifactsBlockOnce(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv.yaml")
	writeFixture(t, configPath, inlineFixtureLF)

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	_, err := configure.Set(deps, paths, []string{"artifacts.commit=never"})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(after), "artifacts:") != 1 {
		t.Errorf("artifacts: key count = %d, want 1", strings.Count(string(after), "artifacts:"))
	}
	if !strings.Contains(string(after), "commit: never") {
		t.Error("commit: never not present")
	}
	if !strings.HasPrefix(string(after), inlineFixtureLF) {
		t.Error("original content was not preserved as a prefix")
	}
}

// ---------------------------------------------------------------------------
// group F (ported as a --set-driven no-op): running --set with a value
// equal to the fixture's own default, against a config that does not exist
// yet, bootstraps and writes; a genuinely empty batch is a pure no-op
// producing no file at all.
// ---------------------------------------------------------------------------

func TestSet_MissingUserConfig_BootstrapsAndWrites(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nerv", "nerv.yaml")

	deps := newTestDeps(dir, time.Now())
	paths := configure.Paths{Config: configPath}

	result, err := configure.Set(deps, paths, []string{"git.worktree=always"})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("Changed = false, want true")
	}
	if result.Backup != nil {
		t.Errorf("Backup = %v, want nil (no prior file)", *result.Backup)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if !strings.Contains(string(after), "git:") || !strings.Contains(string(after), "worktree: always") {
		t.Errorf("bootstrapped content missing the new value: %q", after)
	}
}
