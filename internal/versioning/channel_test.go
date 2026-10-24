package versioning

import (
	"context"
	"testing"
)

// L7, L15: an explicit version takes its channel from the branch's entry,
// never from its prerelease identifier (release/* is rc on the beta
// channel).
func TestBranchChannel_FromTheBranchConfig(t *testing.T) {
	r := newTestRepo(t)
	r.commit("feat: one", "a.txt")
	r.git("branch", "develop")
	r.git("branch", "release/2.0")
	r.git("branch", "topic")
	tests := []struct {
		branch           string
		want             ReleaseBranch
		releaseBranchErr bool
	}{
		{"main", ReleaseBranch{Name: "main"}, false},
		{"develop", ReleaseBranch{Name: "develop", Prerelease: "alpha", Channel: "alpha"}, false},
		{"release/2.0", ReleaseBranch{Name: "release/2.0", Prerelease: "rc", Channel: "beta"}, false},
		{"topic", ReleaseBranch{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			got, err := BranchChannel(context.Background(), Request{RepoDir: r.dir, Branch: tt.branch, Offline: true})
			if tt.releaseBranchErr {
				if err == nil {
					t.Fatalf("BranchChannel = %+v, want an error for a branch that is not a release branch", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("BranchChannel = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}
