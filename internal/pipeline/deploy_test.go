package pipeline

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployResult_WrittenAndRead(t *testing.T) {
	repo := t.TempDir()
	if _, err := ReadDeployResult(repo); !errors.Is(err, ErrNoDeployResult) {
		t.Errorf("no deploy.json: err = %v", err)
	}
	p, err := WriteDeployResult(repo, DeployResult{Step: "deploy", Status: StatusPassed, Environments: []EnvironmentDeploy{{Environment: "staging", Status: DeployDeployed}}})
	if err != nil || p != filepath.Join(repo, ".nerv", "artifacts", "pipeline", "deploy.json") {
		t.Fatalf("WriteDeployResult = %q, %v", p, err)
	}
	if r, err := ReadDeployResult(repo); err != nil || r.Environments[0].Status != "deployed" {
		t.Errorf("ReadDeployResult = %+v, %v", r, err)
	}
}

func TestCheckDeployEvidence_RequiresThisReleasesDockerRecord(t *testing.T) {
	rel := ReleaseOutputs{Status: StatusPassed, Published: true, Version: "1.4.0", GitTag: "v1.4.0", GitHead: "c0ffee"}
	repo := t.TempDir()
	if err := CheckDeployEvidence(repo, rel); !errors.Is(err, ErrNoSyncEvidence) || !strings.HasPrefix(err.Error(), "docker: no docker.json") {
		t.Errorf("no docker.json: err = %v", err)
	}
	if _, err := WriteDockerResult(repo, DockerResult{Status: StatusPassed, Version: "1.3.0", GitTag: "v1.3.0", Revision: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := CheckDeployEvidence(repo, rel); !errors.Is(err, ErrNoSyncEvidence) || !strings.Contains(err.Error(), "not this release") {
		t.Errorf("stale docker.json: err = %v", err)
	}
	if _, err := WriteDockerResult(repo, DockerResult{Status: StatusPassed, Version: "1.4.0", GitTag: "v1.4.0", Revision: "c0ffee"}); err != nil {
		t.Fatal(err)
	}
	// No test record is needed: deploy depends on release and docker only.
	if err := CheckDeployEvidence(repo, rel); err != nil {
		t.Errorf("bound docker.json: %v", err)
	}
}
