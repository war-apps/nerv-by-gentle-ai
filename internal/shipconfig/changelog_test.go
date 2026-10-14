package shipconfig_test

import "testing"

func TestRelease_ChangelogFor(t *testing.T) {
	tests := []struct {
		name, yaml string
		want       map[string]string
	}{
		{"single project, implicit: the repository's own config", `release:
  projects:
    - name: api
      path: .
`, map[string]string{"api": ""}},
		{"single project, explicit", `release:
  projects:
    - name: api
      path: src/Api
      changelog: src/Api/CHANGES.md
`, map[string]string{"api": "src/Api/CHANGES.md"}},
		{"monorepo, implicit and explicit", `release:
  projects:
    - name: api
      path: api
    - name: web
      path: web
      changelog: docs\web-changes.md
`, map[string]string{"api": "api/CHANGELOG.md", "web": "docs/web-changes.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := mustLoad(t, tt.yaml)
			for _, p := range rel.Projects {
				if got := rel.ChangelogFor(p); got != tt.want[p.Name] {
					t.Errorf("ChangelogFor(%s) = %q, want %q", p.Name, got, tt.want[p.Name])
				}
			}
		})
	}
}
