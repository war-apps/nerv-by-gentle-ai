package releaseconfig_test

import "testing"

// Every project's release writes its changelog: the one written in the
// section, or <path>/<changelog.file> (the native engine has no repository
// semantic-release configuration to defer to).
func TestRelease_ProjectChangelog(t *testing.T) {
	tests := []struct {
		name, yaml string
		want       map[string]string
	}{
		{"single project, implicit", `release:
  projects:
    - name: api
      path: .
`, map[string]string{"api": "CHANGELOG.md"}},
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
		{"section file name", `release:
  changelog: { file: HISTORY.md }
  projects:
    - name: api
      path: api
`, map[string]string{"api": "api/HISTORY.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := mustLoad(t, tt.yaml)
			for _, p := range rel.Projects {
				if p.Changelog != tt.want[p.Name] {
					t.Errorf("%s changelog = %q, want %q", p.Name, p.Changelog, tt.want[p.Name])
				}
			}
		})
	}
}
