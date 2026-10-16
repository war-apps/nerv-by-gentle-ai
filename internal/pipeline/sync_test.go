package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
	"github.com/war-apps/nerv-by-gentle-ai/internal/semrel"
)

// fakeSyncRepo is origin's branches and the merges and pushes made.
type fakeSyncRepo struct {
	origin    []string
	conflicts map[string]bool // target -> merging conflicts
	upToDate  map[string]bool
	pushFail  map[string]error
	mergeErr  map[string]error
	merges    []string // "source>target:message"
	previews  []string
	pushes    []string
}

func (f *fakeSyncRepo) Branches(_ context.Context, pattern string) ([]string, error) {
	var out []string
	for _, b := range f.origin {
		if ok, _ := path.Match(pattern, b); ok {
			out = append(out, b)
		}
	}
	slices.Sort(out)
	return out, nil
}

func (f *fakeSyncRepo) outcome(target string) (semrel.MergeResult, error) {
	if err := f.mergeErr[target]; err != nil {
		return semrel.MergeResult{}, err
	}
	switch {
	case f.upToDate[target]:
		return semrel.MergeResult{UpToDate: true}, nil
	case f.conflicts[target]:
		return semrel.MergeResult{Conflict: true, Conflicts: []string{"CHANGELOG.md"}}, nil
	}
	return semrel.MergeResult{}, nil
}

func (f *fakeSyncRepo) Merge(_ context.Context, target, source, message string) (semrel.MergeResult, error) {
	f.merges = append(f.merges, source+">"+target+":"+message)
	r, err := f.outcome(target)
	if err == nil && !r.UpToDate && !r.Conflict {
		r.Commit = "c0ffee" + strings.ReplaceAll(target, "/", "")
	}
	return r, err
}

func (f *fakeSyncRepo) PreviewMerge(_ context.Context, target, source string) (semrel.MergeResult, error) {
	f.previews = append(f.previews, source+">"+target)
	return f.outcome(target)
}

func (f *fakeSyncRepo) PushBranch(_ context.Context, branch string) error {
	if err := f.pushFail[branch]; err != nil {
		return err
	}
	f.pushes = append(f.pushes, branch)
	return nil
}

func syncConfig() *pipelineconfig.Pipeline {
	return &pipelineconfig.Pipeline{
		Branches: []pipelineconfig.Branch{{Name: "main"}, {Name: "develop"}, {Name: "release/*"}, {Name: "hotfix/*"}},
		Sync:     pipelineconfig.Sync{Develop: "develop", Release: "release/*"},
	}
}

func TestSyncTargets_PerReleasedBranch(t *testing.T) {
	origin := []string{"main", "develop", "release/a", "release/b", "hotfix/x", "feature/y"}
	for _, tt := range []struct {
		released string
		want     []string
	}{
		{"main", []string{"main>develop", "main>release/a", "main>release/b"}},
		{"release/a", []string{"release/a>develop"}},
		{"develop", nil},
		{"hotfix/x", nil},
	} {
		f := &fakeSyncRepo{origin: origin}
		targets, err := Sync{Repo: f, Config: syncConfig(), Release: ReleaseOutputs{Branch: tt.released}}.Targets(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, tg := range targets {
			got = append(got, tg.Source+">"+tg.Branch)
			if want := fmt.Sprintf("chore(sync): merge %s into %s [skip ci]", tg.Source, tg.Branch); tg.Message != want {
				t.Errorf("message %q, want %q", tg.Message, want)
			}
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("released %s: targets %q, want %q", tt.released, got, tt.want)
		}
	}
}

func TestSyncTargets_ConfiguredBranches(t *testing.T) {
	cfg := syncConfig()
	cfg.Branches = []pipelineconfig.Branch{{Name: "master"}, {Name: "next"}, {Name: "rc/*"}}
	cfg.Sync = pipelineconfig.Sync{Develop: "next", Release: "rc/*"}
	f := &fakeSyncRepo{origin: []string{"master", "next", "rc/1", "release/old"}}
	targets, err := Sync{Repo: f, Config: cfg, Release: ReleaseOutputs{Branch: "master"}}.Targets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].Branch != "next" || targets[1].Branch != "rc/1" {
		t.Errorf("targets %+v", targets)
	}
}

func TestSync_ConflictIsAbortedAndTheOthersContinue(t *testing.T) {
	f := &fakeSyncRepo{origin: []string{"main", "develop", "release/a", "release/b", "release/c"},
		conflicts: map[string]bool{"release/a": true}, upToDate: map[string]bool{"release/c": true}}
	var out bytes.Buffer
	s := Sync{Repo: f, Config: syncConfig(), Release: ReleaseOutputs{Branch: "main", Version: "1.2.0"}, Output: &out}
	targets, err := s.Targets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	res := s.Run(context.Background(), targets)
	if res.Status != StatusFailed || !strings.Contains(res.Error, "release/a") {
		t.Errorf("status %s error %q, want failed naming release/a", res.Status, res.Error)
	}
	want := map[string]string{"develop": "pushed", "release/a": "conflict", "release/b": "pushed", "release/c": "up-to-date"}
	if len(res.Branches) != len(want) {
		t.Fatalf("branches %+v", res.Branches)
	}
	for _, b := range res.Branches {
		if b.Status != want[b.Branch] || b.Source != "main" {
			t.Errorf("%s: %+v, want %s from main", b.Branch, b, want[b.Branch])
		}
		if (b.Status == "pushed") != (b.Commit != "") {
			t.Errorf("%s: commit %q with status %s", b.Branch, b.Commit, b.Status)
		}
	}
	if !slices.Equal(res.Branches[1].Conflicts, []string{"CHANGELOG.md"}) {
		t.Errorf("conflicts %v", res.Branches[1].Conflicts)
	}
	if !slices.Equal(f.pushes, []string{"develop", "release/b"}) {
		t.Errorf("pushes %q", f.pushes)
	}
	if len(f.merges) != 4 || f.merges[0] != "main>develop:chore(sync): merge main into develop [skip ci]" {
		t.Errorf("merges %q", f.merges)
	}
	if !strings.Contains(out.String(), "release/a: conflict") {
		t.Errorf("output %q", out.String())
	}
}

func TestSync_FailuresAreRecordedPerBranch(t *testing.T) {
	f := &fakeSyncRepo{origin: []string{"main", "release/a"},
		mergeErr: map[string]error{"develop": fmt.Errorf("develop: %w", semrel.ErrBranchNotOnOrigin)},
		pushFail: map[string]error{"release/a": errors.New("rejected: non-fast-forward")}}
	s := Sync{Repo: f, Config: syncConfig(), Release: ReleaseOutputs{Branch: "main"}}
	targets, _ := s.Targets(context.Background())
	res := s.Run(context.Background(), targets)
	if res.Status != StatusFailed || len(res.Branches) != 2 {
		t.Fatalf("result %+v", res)
	}
	d, a := res.Branches[0], res.Branches[1]
	if d.Status != "failed" || !strings.Contains(d.Error, "not on origin") {
		t.Errorf("develop %+v", d)
	}
	if a.Status != "failed" || a.Commit == "" || !strings.Contains(a.Error, "non-fast-forward") {
		t.Errorf("release/a %+v: a failed push keeps the local merge commit and the cause", a)
	}
	if !strings.Contains(res.Error, "develop") || !strings.Contains(res.Error, "release/a") {
		t.Errorf("error %q", res.Error)
	}
}

func TestSync_DryRunPreviewsWithoutMergingOrPushing(t *testing.T) {
	f := &fakeSyncRepo{origin: []string{"main", "develop", "release/a", "release/b"},
		conflicts: map[string]bool{"release/a": true}, upToDate: map[string]bool{"release/b": true}}
	s := Sync{Repo: f, Config: syncConfig(), Release: ReleaseOutputs{Branch: "main"}, DryRun: true}
	targets, _ := s.Targets(context.Background())
	res := s.Run(context.Background(), targets)
	if len(f.merges) != 0 || len(f.pushes) != 0 || len(f.previews) != 3 {
		t.Errorf("merges %q pushes %q previews %q", f.merges, f.pushes, f.previews)
	}
	if res.Status != StatusPassed || !res.DryRun {
		t.Errorf("status %s: a dry run reports conflicts without failing", res.Status)
	}
	want := []string{"would-merge", "would-conflict", "up-to-date"}
	for i, b := range res.Branches {
		if b.Status != want[i] {
			t.Errorf("%s: %s, want %s", b.Branch, b.Status, want[i])
		}
	}
}

func TestSync_InterruptedStopsBeforeTheNextBranch(t *testing.T) {
	f := &fakeSyncRepo{origin: []string{"main", "develop", "release/a"}}
	s := Sync{Repo: f, Config: syncConfig(), Release: ReleaseOutputs{Branch: "main"}}
	targets, _ := s.Targets(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := s.Run(ctx, targets)
	if len(f.merges) != 0 || res.Status != StatusInterrupted {
		t.Errorf("merges %q status %s", f.merges, res.Status)
	}
}

func TestSyncAndDockerArtifacts(t *testing.T) {
	repo := t.TempDir()
	if _, err := ReadDockerResult(repo); !errors.Is(err, ErrNoDockerResult) {
		t.Errorf("no docker.json: err = %v", err)
	}
	if _, err := WriteDockerResult(repo, DockerResult{Step: "docker", Status: StatusFailed, Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if d, err := ReadDockerResult(repo); err != nil || d.Status != StatusFailed || d.Version != "1.0.0" {
		t.Errorf("ReadDockerResult = %+v, %v", d, err)
	}
	p, err := WriteSyncResult(repo, SyncResult{Step: "sync", Status: StatusPassed, Branches: []BranchSync{{Branch: "develop", Status: "pushed"}}})
	if err != nil || p != SyncResultPath(repo) || p != filepath.Join(repo, ".nerv", "artifacts", "pipeline", "sync.json") {
		t.Fatalf("WriteSyncResult = %q, %v", p, err)
	}
	if err := RemoveSyncResult(repo); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSyncResult(repo); err != nil {
		t.Errorf("removing a missing sync.json: %v", err)
	}
}
