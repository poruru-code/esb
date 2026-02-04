// Where: cli/internal/domain/runtime/mode_test.go
// What: Unit tests for runtime mode helpers.
// Why: Keep mode inference stable as command/usecase layers evolve.
package runtime

import "testing"

func TestNormalizeMode(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "docker", want: ModeDocker},
		{input: "Containerd", want: ModeContainerd},
		{input: "  docker  ", want: ModeDocker},
		{input: "unknown", wantErr: true},
	}
	for _, tc := range cases {
		got, err := NormalizeMode(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("expected error for %q", tc.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("NormalizeMode(%q)=%q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFallbackMode(t *testing.T) {
	if got := FallbackMode(""); got != ModeDocker {
		t.Fatalf("FallbackMode(empty)=%q, want %q", got, ModeDocker)
	}
	if got := FallbackMode("containerd"); got != ModeContainerd {
		t.Fatalf("FallbackMode(containerd)=%q, want %q", got, ModeContainerd)
	}
	if got := FallbackMode("invalid"); got != ModeDocker {
		t.Fatalf("FallbackMode(invalid)=%q, want %q", got, ModeDocker)
	}
}

func TestInferModeFromComposeFiles(t *testing.T) {
	files := []string{
		"/tmp/docker-compose.docker.yml",
		"/tmp/docker-compose.containerd.yml",
	}
	if got := InferModeFromComposeFiles(files); got != ModeContainerd {
		t.Fatalf("expected containerd, got %q", got)
	}
	files = []string{"/tmp/docker-compose.docker.yml"}
	if got := InferModeFromComposeFiles(files); got != ModeDocker {
		t.Fatalf("expected docker, got %q", got)
	}
	files = []string{"/tmp/docker-compose.yml"}
	if got := InferModeFromComposeFiles(files); got != ModeDocker {
		t.Fatalf("expected docker, got %q", got)
	}
}

func TestInferModeFromContainers(t *testing.T) {
	containers := []ContainerInfo{
		{Service: "agent", State: "running"},
		{Service: "runtime-node", State: "running"},
	}
	if got := InferModeFromContainers(containers, true); got != ModeContainerd {
		t.Fatalf("expected containerd, got %q", got)
	}
	containers = []ContainerInfo{
		{Service: "runtime-node", State: "exited"},
		{Service: "agent", State: "running"},
	}
	if got := InferModeFromContainers(containers, true); got != ModeDocker {
		t.Fatalf("expected docker, got %q", got)
	}
	if got := InferModeFromContainers(containers, false); got != ModeContainerd {
		t.Fatalf("expected containerd when runningOnly=false, got %q", got)
	}
}
