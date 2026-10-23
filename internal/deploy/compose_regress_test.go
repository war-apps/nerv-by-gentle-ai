package deploy

import (
	"strings"
	"testing"
)

// The default target selection refuses an environment whose
// DOCKER_IMAGE_VERSION is not set, so an
// alpha never reaches an environment that names no version.
func TestRunComposeOp_RefusesAMissingImageVersion(t *testing.T) {
	f := &fakeProc{}
	resp, code := runCompose(t, "deploy", requestFor("deploy", nil), f, stagingEnv(nil))
	if code != 1 || resp["status"] != "failed" || !strings.Contains(resp["message"], "DOCKER_IMAGE_VERSION is not set") || len(f.cmds) != 0 {
		t.Errorf("no DOCKER_IMAGE_VERSION: %v code %d, %d ssh commands", resp, code, len(f.cmds))
	}
}
