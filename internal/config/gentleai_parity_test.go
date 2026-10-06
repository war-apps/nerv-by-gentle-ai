package config_test

import (
	"slices"
	"testing"

	"github.com/war-apps/nerv-by-gentle-ai/internal/config"
)

// Every gentle-ai v4 agent must be claimed by at least one NERV role, so
// running NERV never loses a gentle-ai capability.
func TestGentleAIV4Agents_EveryAgentIsClaimedByARole(t *testing.T) {
	cat := config.Roles()
	for _, agent := range config.GentleAIV4Agents {
		var claimedBy []string
		for _, role := range cat.AllRoles {
			if slices.Contains(cat.Info[role].GentleAIEquivalents, agent) {
				claimedBy = append(claimedBy, role)
			}
		}
		if len(claimedBy) == 0 {
			t.Errorf("gentle-ai v4 agent %q is claimed by no NERV role", agent)
		}
	}
}

// Every claimed equivalent must be a real gentle-ai v4 agent, so a typo or
// a removed agent cannot pass the coverage check above.
func TestGentleAIV4Agents_EveryClaimIsAV4Agent(t *testing.T) {
	cat := config.Roles()
	for _, role := range cat.AllRoles {
		seen := map[string]bool{}
		for _, agent := range cat.Info[role].GentleAIEquivalents {
			if !slices.Contains(config.GentleAIV4Agents, agent) {
				t.Errorf("role %q claims %q, which is not a gentle-ai v4 agent", role, agent)
			}
			if seen[agent] {
				t.Errorf("role %q claims %q twice", role, agent)
			}
			seen[agent] = true
		}
	}
}

func TestGentleAIV4Agents_ListIsTheV4Set(t *testing.T) {
	want := []string{
		"jd-fix-agent", "jd-judge-a", "jd-judge-b", "review-readability",
		"review-refuter", "review-reliability", "review-resilience", "review-risk",
	}
	got := slices.Sorted(slices.Values(config.GentleAIV4Agents))
	if !slices.Equal(got, want) {
		t.Errorf("GentleAIV4Agents = %v, want %v", got, want)
	}
}
