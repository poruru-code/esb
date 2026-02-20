package artifactcore

import "testing"

func TestInferRuntimeModeFromServiceImages(t *testing.T) {
	if got := InferRuntimeModeFromServiceImages(map[string]string{
		"gateway": "registry:5010/esb-gateway-docker:latest",
	}); got != "docker" {
		t.Fatalf("docker mode = %q", got)
	}
	if got := InferRuntimeModeFromServiceImages(map[string]string{
		"gateway": "registry:5010/esb-gateway-containerd:latest",
	}); got != "containerd" {
		t.Fatalf("containerd mode = %q", got)
	}
	if got := InferRuntimeModeFromServiceImages(map[string]string{
		"gateway": "public.ecr.aws/lambda/python:3.12",
	}); got != "" {
		t.Fatalf("unknown mode = %q", got)
	}
}

func TestInferRuntimeModeFromImageRefsPrefersContainerd(t *testing.T) {
	got := InferRuntimeModeFromImageRefs([]string{
		"registry:5010/esb-gateway-docker:latest",
		"registry:5010/esb-agent-containerd:latest",
	})
	if got != "containerd" {
		t.Fatalf("mixed mode = %q", got)
	}
}

func TestParseRuntimeImageTag(t *testing.T) {
	cases := []struct {
		name  string
		image string
		want  string
	}{
		{name: "registry with port", image: "registry:5010/esb-gateway-docker:latest", want: "latest"},
		{name: "digest", image: "registry:5010/esb-gateway-docker:latest@sha256:abc", want: "latest"},
		{name: "no tag", image: "registry:5010/esb-gateway-docker", want: ""},
		{name: "empty", image: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseRuntimeImageTag(tc.image); got != tc.want {
				t.Fatalf("ParseRuntimeImageTag(%q) = %q, want %q", tc.image, got, tc.want)
			}
		})
	}
}
