package deploy

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/war-apps/nerv-by-gentle-ai/internal/pipeline"
	"github.com/war-apps/nerv-by-gentle-ai/internal/pipelineconfig"
)

// Decision says whether a release deploys an environment, and why not.
type Decision struct {
	Deploy bool
	Reason string
}

var semverRe = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+`)

// Select decides whether the release rel deploys the target:
//   - DOCKER_DEPLOY_ENABLED false skips it;
//   - selection target: when DOCKER_IMAGE_VERSION points at the release, by
//     update-deploy's resolve_target rules applied to the release: a version
//     equal to the released one (a leading v ignored); latest, a release
//     without prerelease; alpha, a version with -alpha.; beta, one with
//     -rc. or -beta.; any other tag, one with -<tag>. (update-deploy's
//     fallback of an unknown tag to the latest release needs the list of
//     releases and is not applied). No DOCKER_IMAGE_VERSION is an error;
//   - selection branches: when the environment's branches match the
//     released branch.
func Select(t Target, env pipelineconfig.Environment, selection string, rel pipeline.ReleaseOutputs) (Decision, error) {
	if !t.Enabled {
		return Decision{Reason: "DOCKER_DEPLOY_ENABLED is off"}, nil
	}
	if selection == "branches" {
		if env.Matches(rel.Branch) {
			return Decision{Deploy: true}, nil
		}
		return Decision{Reason: fmt.Sprintf("its branches (%s) do not match the released branch %s", strings.Join(env.Branches, ", "), rel.Branch)}, nil
	}
	v := t.ImageVersion
	if v == "" {
		return Decision{}, fmt.Errorf("%s: DOCKER_IMAGE_VERSION is not set: with selection target it says which releases the environment runs", t.Environment)
	}
	version := strings.TrimPrefix(rel.Version, "v")
	var ok bool
	switch {
	case semverRe.MatchString(v):
		ok = strings.TrimPrefix(v, "v") == version
	case v == "latest":
		ok = !strings.Contains(version, "-")
	case v == "alpha":
		ok = strings.Contains(version, "-alpha.")
	case v == "beta":
		ok = strings.Contains(version, "-rc.") || strings.Contains(version, "-beta.")
	default:
		ok = strings.Contains(version, "-"+v+".")
	}
	if ok {
		return Decision{Deploy: true}, nil
	}
	return Decision{Reason: fmt.Sprintf("DOCKER_IMAGE_VERSION %s does not point at %s", v, rel.Version)}, nil
}
