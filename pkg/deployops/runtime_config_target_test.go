package deployops

import (
	"errors"
	"strings"
	"testing"
)

type fakeDockerResponse struct {
	output string
	err    error
}

type fakeDockerRunner struct {
	responses map[string]fakeDockerResponse
}

func (f fakeDockerRunner) run(args ...string) ([]byte, error) {
	key := strings.Join(args, "\x00")
	resp, ok := f.responses[key]
	if !ok {
		return nil, errors.New("unexpected docker command: " + strings.Join(args, " "))
	}
	if resp.err != nil {
		return nil, resp.err
	}
	return []byte(resp.output), nil
}

func TestDockerRuntimeConfigResolverResolveWithProjectGatewayVolume(t *testing.T) {
	runner := fakeDockerRunner{
		responses: map[string]fakeDockerResponse{
			"ps\x00-q\x00--filter\x00label=com.docker.compose.project=esb-e2e-docker\x00--filter\x00label=com.docker.compose.service=gateway": {
				output: "gateway-id\n",
			},
			"inspect\x00gateway-id": {
				output: `[{"Mounts":[{"Destination":"/app/runtime-config","Type":"volume","Name":"esb-runtime-config","Source":"/var/lib/docker/volumes/esb-runtime-config/_data"}]}]`,
			},
		},
	}
	resolver := dockerRuntimeConfigResolver{
		projectName: "esb-e2e-docker",
		runDocker:   runner.run,
	}

	resolved, err := resolver.ResolveRuntimeConfigTarget()
	if err != nil {
		t.Fatalf("ResolveRuntimeConfigTarget() error = %v", err)
	}
	if resolved.VolumeName != "esb-runtime-config" {
		t.Fatalf("ResolveRuntimeConfigTarget() = %#v, want volume target", resolved)
	}
}

func TestDockerRuntimeConfigResolverFallsBackToProvisionerBindPath(t *testing.T) {
	runner := fakeDockerRunner{
		responses: map[string]fakeDockerResponse{
			"ps\x00-q\x00--filter\x00label=com.docker.compose.project=esb-e2e-docker\x00--filter\x00label=com.docker.compose.service=gateway": {
				output: "gateway-id\n",
			},
			"inspect\x00gateway-id": {
				output: `[{"Mounts":[{"Destination":"/app/other","Source":"/tmp/other"}]}]`,
			},
			"ps\x00-q\x00--filter\x00label=com.docker.compose.project=esb-e2e-docker\x00--filter\x00label=com.docker.compose.service=provisioner": {
				output: "provisioner-id\n",
			},
			"inspect\x00provisioner-id": {
				output: `[{"Mounts":[{"Destination":"/app/runtime-config","Type":"bind","Source":"/tmp/runtime-config"}]}]`,
			},
		},
	}
	resolver := dockerRuntimeConfigResolver{
		projectName: "esb-e2e-docker",
		runDocker:   runner.run,
	}

	resolved, err := resolver.ResolveRuntimeConfigTarget()
	if err != nil {
		t.Fatalf("ResolveRuntimeConfigTarget() error = %v", err)
	}
	if resolved.BindPath != "/tmp/runtime-config" {
		t.Fatalf("ResolveRuntimeConfigTarget() = %#v, want bind target", resolved)
	}
}

func TestDockerRuntimeConfigResolverWithoutProjectFallsBackToProvisionerBindPath(t *testing.T) {
	runner := fakeDockerRunner{
		responses: map[string]fakeDockerResponse{
			"ps\x00-q\x00--filter\x00label=com.docker.compose.service=gateway": {
				output: "gateway-id\n",
			},
			"inspect\x00gateway-id": {
				output: `[{"Mounts":[{"Destination":"/app/other","Source":"/tmp/other"}]}]`,
			},
			"ps\x00-q\x00--filter\x00label=com.docker.compose.service=provisioner": {
				output: "provisioner-id\n",
			},
			"inspect\x00provisioner-id": {
				output: `[{"Mounts":[{"Destination":"/app/runtime-config","Type":"bind","Source":"/tmp/runtime-config"}]}]`,
			},
		},
	}
	resolver := dockerRuntimeConfigResolver{
		runDocker: runner.run,
	}

	resolved, err := resolver.ResolveRuntimeConfigTarget()
	if err != nil {
		t.Fatalf("ResolveRuntimeConfigTarget() error = %v", err)
	}
	if resolved.BindPath != "/tmp/runtime-config" {
		t.Fatalf("ResolveRuntimeConfigTarget() = %#v, want bind target", resolved)
	}
}

func TestDockerRuntimeConfigResolverRequiresProjectWhenAmbiguous(t *testing.T) {
	runner := fakeDockerRunner{
		responses: map[string]fakeDockerResponse{
			"ps\x00-q\x00--filter\x00label=com.docker.compose.service=gateway": {
				output: "gateway-1\ngateway-2\n",
			},
		},
	}
	resolver := dockerRuntimeConfigResolver{
		runDocker: runner.run,
	}

	_, err := resolver.ResolveRuntimeConfigTarget()
	if err == nil {
		t.Fatal("ResolveRuntimeConfigTarget() expected error")
	}
	if !strings.Contains(err.Error(), "multiple running gateway containers") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDockerRuntimeConfigResolverRequiresRunningComposeContainer(t *testing.T) {
	runner := fakeDockerRunner{
		responses: map[string]fakeDockerResponse{
			"ps\x00-q\x00--filter\x00label=com.docker.compose.service=gateway":     {},
			"ps\x00-q\x00--filter\x00label=com.docker.compose.service=provisioner": {},
		},
	}
	resolver := dockerRuntimeConfigResolver{
		runDocker: runner.run,
	}

	_, err := resolver.ResolveRuntimeConfigTarget()
	if err == nil {
		t.Fatal("ResolveRuntimeConfigTarget() expected error")
	}
	if !strings.Contains(err.Error(), "no running gateway/provisioner compose container found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
