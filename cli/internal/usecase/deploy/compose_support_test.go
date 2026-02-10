// Where: cli/internal/usecase/deploy/compose_support_test.go
// What: Unit tests for compose support wrappers in deploy workflow.
// Why: Ensure usecase layer delegates compose operations to infra provisioner.
package deploy

import (
	"errors"
	"testing"

	infradeploy "github.com/poruru/edge-serverless-box/cli/internal/infra/deploy"
)

type recordProvisioner struct {
	statusCalls int
	statusReq   struct {
		project string
		mode    string
	}
	runCalls int
	runReq   infradeploy.ProvisionerRequest
	runErr   error
}

func (p *recordProvisioner) CheckServicesStatus(composeProject, mode string) {
	p.statusCalls++
	p.statusReq.project = composeProject
	p.statusReq.mode = mode
}

func (p *recordProvisioner) RunProvisioner(request infradeploy.ProvisionerRequest) error {
	p.runCalls++
	p.runReq = request
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
	if provisioner.runReq.ComposeProject != "esb-dev" {
		t.Fatalf("compose project mismatch: %q", provisioner.runReq.ComposeProject)
	}
	if provisioner.runReq.Mode != "containerd" {
		t.Fatalf("mode mismatch: %q", provisioner.runReq.Mode)
	}
	if !provisioner.runReq.NoDeps {
		t.Fatalf("expected noDeps=true")
	}
	if !provisioner.runReq.Verbose {
		t.Fatalf("expected verbose=true")
	}
	if provisioner.runReq.ProjectDir != "/tmp/project" {
		t.Fatalf("project dir mismatch: %q", provisioner.runReq.ProjectDir)
	}
	if len(provisioner.runReq.ComposeFiles) != 1 || provisioner.runReq.ComposeFiles[0] != "/tmp/docker-compose.yml" {
		t.Fatalf("compose files mismatch: %#v", provisioner.runReq.ComposeFiles)
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
