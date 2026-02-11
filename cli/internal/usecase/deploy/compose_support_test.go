// Where: cli/internal/usecase/deploy/compose_support_test.go
// What: Unit tests for compose support wrappers in deploy workflow.
// Why: Ensure usecase layer delegates compose operations to infra provisioner.
package deploy

import (
	"errors"
	"testing"
)

type recordProvisioner struct {
	statusCalls int
	statusReq   struct {
		project string
		mode    string
	}
	runCalls int
	runReq   struct {
		project      string
		mode         string
		noDeps       bool
		verbose      bool
		projectDir   string
		composeFiles []string
	}
	runErr error
}

func (p *recordProvisioner) CheckServicesStatus(composeProject, mode string) {
	p.statusCalls++
	p.statusReq.project = composeProject
	p.statusReq.mode = mode
}

func (p *recordProvisioner) RunProvisioner(
	composeProject string,
	mode string,
	noDeps bool,
	verbose bool,
	projectDir string,
	composeFiles []string,
) error {
	p.runCalls++
	p.runReq.project = composeProject
	p.runReq.mode = mode
	p.runReq.noDeps = noDeps
	p.runReq.verbose = verbose
	p.runReq.projectDir = projectDir
	p.runReq.composeFiles = append([]string{}, composeFiles...)
	return p.runErr
}

func TestCheckServicesStatusDelegatesToProvisioner(t *testing.T) {
	provisioner := &recordProvisioner{}
	workflow := Workflow{ComposeProvisioner: provisioner}

	workflow.checkServicesStatus("esb-dev", "docker")

	if provisioner.statusCalls != 1 {
		t.Fatalf("expected one status call, got %d", provisioner.statusCalls)
	}
	if provisioner.statusReq.project != "esb-dev" {
		t.Fatalf("project mismatch: %q", provisioner.statusReq.project)
	}
	if provisioner.statusReq.mode != "docker" {
		t.Fatalf("mode mismatch: %q", provisioner.statusReq.mode)
	}
}

func TestRunProvisionerDelegatesRequest(t *testing.T) {
	provisioner := &recordProvisioner{}
	workflow := Workflow{ComposeProvisioner: provisioner}

	err := workflow.runProvisioner(
		"esb-dev",
		"containerd",
		true,
		true,
		"/tmp/project",
		[]string{"/tmp/docker-compose.yml"},
	)
	if err != nil {
		t.Fatalf("runProvisioner: %v", err)
	}
	if provisioner.runCalls != 1 {
		t.Fatalf("expected one run call, got %d", provisioner.runCalls)
	}
	if provisioner.runReq.project != "esb-dev" {
		t.Fatalf("compose project mismatch: %q", provisioner.runReq.project)
	}
	if provisioner.runReq.mode != "containerd" {
		t.Fatalf("mode mismatch: %q", provisioner.runReq.mode)
	}
	if !provisioner.runReq.noDeps {
		t.Fatalf("expected noDeps=true")
	}
	if !provisioner.runReq.verbose {
		t.Fatalf("expected verbose=true")
	}
	if provisioner.runReq.projectDir != "/tmp/project" {
		t.Fatalf("project dir mismatch: %q", provisioner.runReq.projectDir)
	}
	if len(provisioner.runReq.composeFiles) != 1 || provisioner.runReq.composeFiles[0] != "/tmp/docker-compose.yml" {
		t.Fatalf("compose files mismatch: %#v", provisioner.runReq.composeFiles)
	}
}

func TestRunProvisionerPropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	provisioner := &recordProvisioner{runErr: wantErr}
	workflow := Workflow{ComposeProvisioner: provisioner}

	err := workflow.runProvisioner("esb-dev", "docker", false, false, "/tmp/project", nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected propagated error, got %v", err)
	}
}
