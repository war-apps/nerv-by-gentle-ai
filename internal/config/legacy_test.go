package config_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

func TestRemovedTaskProviders_ListsTheTwoRemovedProviders(t *testing.T) {
	want := []string{"github-projects", "jira"}
	got := slices.Clone(config.RemovedTaskProviders)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("RemovedTaskProviders = %v, want %v", config.RemovedTaskProviders, want)
	}
	// A removed provider must never be accepted again as a tasks.provider value.
	allowed, _ := config.AllowedValues("tasks.provider")
	for _, name := range config.RemovedTaskProviders {
		if slices.Contains(allowed, name) {
			t.Errorf("%q is both removed and allowed", name)
		}
	}
}

const (
	stripHead = "" +
		"enabled: true\n" +
		"git:\n" +
		"  base_branch: develop              # default base\n" +
		"tasks:\n"
	stripTeamwork = "" +
		"  providers:                        # one block per provider\n" +
		"    teamwork:\n" +
		"      task_ref_prefix: tw           # prefix\n" +
		"      stages: { inDev: DESARROLLO }\n"
	stripGithubInline = "    github-projects: { task_ref_prefix: gh, owner: \"\", project_number: 0 }    # later\n"
	stripJiraInline   = "    jira: { task_ref_prefix: jira, site: \"\", project_key: \"\" }               # later\n"
	stripJiraMulti    = "" +
		"    jira:                           # stub\n" +
		"      task_ref_prefix: jira\n" +
		"      # which site\n" +
		"      site: \"\"\n" +
		"\n" +
		"      project_key: \"\"\n"
	stripGithubMulti = "" +
		"    github-projects:\n" +
		"      task_ref_prefix: gh\n" +
		"      owner: \"\"\n"
	stripTail = "" +
		"  sources:                          # extra work sources\n" +
		"    - name: erp\n" +
		"critical_paths: [auth/]\n"
)

func TestStripRemovedTaskProviders(t *testing.T) {
	const (
		provLine  = "tasks.provider (jira)"
		githubKey = "tasks.providers.github-projects"
		jiraKey   = "tasks.providers.jira"
	)
	cases := []struct {
		name        string
		in          string
		want        string
		wantRemoved []string
	}{
		{
			name:        "inline blocks mid-file",
			in:          stripHead + "  provider: teamwork\n" + stripTeamwork + stripGithubInline + stripJiraInline + stripTail,
			want:        stripHead + "  provider: teamwork\n" + stripTeamwork + stripTail,
			wantRemoved: []string{githubKey, jiraKey},
		},
		{
			name:        "inline blocks at EOF",
			in:          stripHead + stripTeamwork + stripGithubInline + stripJiraInline,
			want:        stripHead + stripTeamwork,
			wantRemoved: []string{githubKey, jiraKey},
		},
		{
			name:        "multi-line blocks mid-file",
			in:          stripHead + stripTeamwork + stripGithubMulti + stripJiraMulti + stripTail,
			want:        stripHead + stripTeamwork + stripTail,
			wantRemoved: []string{githubKey, jiraKey},
		},
		{
			name:        "multi-line block at EOF",
			in:          stripHead + stripTeamwork + stripJiraMulti,
			want:        stripHead + stripTeamwork,
			wantRemoved: []string{jiraKey},
		},
		{
			name:        "only jira present",
			in:          stripHead + stripTeamwork + stripJiraInline + stripTail,
			want:        stripHead + stripTeamwork + stripTail,
			wantRemoved: []string{jiraKey},
		},
		{
			name:        "only github-projects present",
			in:          stripHead + stripTeamwork + stripGithubInline + stripTail,
			want:        stripHead + stripTeamwork + stripTail,
			wantRemoved: []string{githubKey},
		},
		{
			name:        "provider line with a trailing comment",
			in:          stripHead + "  provider: jira                    # teamwork | none\n  ask_when_missing: true\n" + stripTeamwork,
			want:        stripHead + "  ask_when_missing: true\n" + stripTeamwork,
			wantRemoved: []string{provLine},
		},
		{
			name:        "provider line without a comment",
			in:          stripHead + "  provider: jira\n  ask_when_missing: true\n",
			want:        stripHead + "  ask_when_missing: true\n",
			wantRemoved: []string{provLine},
		},
		{
			name:        "quoted github-projects provider value",
			in:          stripHead + "  provider: \"github-projects\"\n  ask_when_missing: true\n",
			want:        stripHead + "  ask_when_missing: true\n",
			wantRemoved: []string{"tasks.provider (github-projects)"},
		},
		{
			name:        "provider line and both blocks together",
			in:          stripHead + "  provider: jira   # legacy\n" + stripTeamwork + stripGithubInline + stripJiraMulti + stripTail,
			want:        stripHead + stripTeamwork + stripTail,
			wantRemoved: []string{provLine, githubKey, jiraKey},
		},
		{
			name: "comment lines right above a block go with it",
			in: stripHead + stripTeamwork +
				"    # legacy stubs, kept by hand\n" + stripGithubInline + stripJiraInline + stripTail,
			want:        stripHead + stripTeamwork + stripTail,
			wantRemoved: []string{githubKey, jiraKey},
		},
		{
			name: "a comment separated by a blank line stays",
			in: stripHead + stripTeamwork +
				"    # about the next one\n\n" + stripJiraInline + stripTail,
			want:        stripHead + stripTeamwork + "    # about the next one\n\n" + stripTail,
			wantRemoved: []string{jiraKey},
		},
		{
			name:        "CRLF line endings are kept",
			in:          strings.ReplaceAll(stripHead+"  provider: jira\n"+stripTeamwork+stripGithubInline+stripJiraMulti+stripTail, "\n", "\r\n"),
			want:        strings.ReplaceAll(stripHead+stripTeamwork+stripTail, "\n", "\r\n"),
			wantRemoved: []string{provLine, githubKey, jiraKey},
		},
		{
			name:        "other providers and similar keys are untouched",
			in:          stripHead + stripTeamwork + "    jira-cloud: { site: x }\n    jirax:\n      a: 1\n" + stripTail,
			want:        stripHead + stripTeamwork + "    jira-cloud: { site: x }\n    jirax:\n      a: 1\n" + stripTail,
			wantRemoved: nil,
		},
		{
			name:        "provider and jira keys outside tasks are untouched",
			in:          "other:\n  provider: jira\n  providers:\n    jira: { a: 1 }\n" + stripHead + stripTeamwork,
			want:        "other:\n  provider: jira\n  providers:\n    jira: { a: 1 }\n" + stripHead + stripTeamwork,
			wantRemoved: nil,
		},
		{
			name:        "teamwork provider is untouched",
			in:          stripHead + "  provider: teamwork                # teamwork | none\n" + stripTeamwork,
			want:        stripHead + "  provider: teamwork                # teamwork | none\n" + stripTeamwork,
			wantRemoved: nil,
		},
		{
			name:        "none provider is untouched",
			in:          stripHead + "  provider: none\n" + stripTeamwork,
			want:        stripHead + "  provider: none\n" + stripTeamwork,
			wantRemoved: nil,
		},
		{
			name:        "clean file is untouched",
			in:          stripHead + stripTeamwork + stripTail,
			want:        stripHead + stripTeamwork + stripTail,
			wantRemoved: nil,
		},
		{
			name:        "file without a tasks block is untouched",
			in:          "enabled: true\ngit:\n  base_branch: develop\n",
			want:        "enabled: true\ngit:\n  base_branch: develop\n",
			wantRemoved: nil,
		},
		{
			name:        "empty input",
			in:          "",
			want:        "",
			wantRemoved: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, removed := config.StripRemovedTaskProviders([]byte(tc.in))
			if string(got) != tc.want {
				t.Errorf("output mismatch\ngot:\n%q\nwant:\n%q", got, tc.want)
			}
			if !slices.Equal(removed, tc.wantRemoved) {
				t.Errorf("removed = %v, want %v", removed, tc.wantRemoved)
			}
		})
	}
}

// The stripped output is a fixed point: running it again removes nothing.
func TestStripRemovedTaskProviders_Idempotent(t *testing.T) {
	in := stripHead + "  provider: jira\n" + stripTeamwork + stripGithubInline + stripJiraMulti + stripTail
	once, _ := config.StripRemovedTaskProviders([]byte(in))
	twice, removed := config.StripRemovedTaskProviders(once)
	if string(twice) != string(once) || len(removed) != 0 {
		t.Errorf("second pass changed the file or reported %v:\n%s", removed, twice)
	}
}
