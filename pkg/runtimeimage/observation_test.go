package runtimeimage

import "testing"

func TestInferModeFromServiceImages(t *testing.T) {
	if got := InferModeFromServiceImages(map[string]string{
		"gateway": "registry:5010/esb-gateway-docker:latest",
	}); got != "docker" {
		t.Fatalf("InferModeFromServiceImages() docker = %q", got)
	}
	if got := InferModeFromServiceImages(map[string]string{
		"gateway": "registry:5010/esb-gateway-containerd:latest",
	}); got != "containerd" {
		t.Fatalf("InferModeFromServiceImages() containerd = %q", got)
	}
	if got := InferModeFromServiceImages(map[string]string{
		"gateway": "public.ecr.aws/lambda/python:3.12",
	}); got != "" {
		t.Fatalf("InferModeFromServiceImages() unknown = %q", got)
	}
}

func TestInferModeFromImageRefsPrefersContainerd(t *testing.T) {
	refs := []string{
		"registry:5010/esb-agent-docker:latest",
		"registry:5010/esb-gateway-containerd:latest",
	}
	if got := InferModeFromImageRefs(refs); got != "containerd" {
		t.Fatalf("InferModeFromImageRefs() = %q, want containerd", got)
	}
}

func TestParseTag(t *testing.T) {
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
			if got := ParseTag(tc.image); got != tc.want {
				t.Fatalf("ParseTag(%q) = %q, want %q", tc.image, got, tc.want)
			}
		})
	}
}

func TestPreferredServiceImage(t *testing.T) {
	imageRef, service := PreferredServiceImage(map[string]string{
		"scheduler": "registry:5010/esb-scheduler-docker:latest",
		"gateway":   "registry:5010/esb-gateway-docker:latest",
	})
	if imageRef != "registry:5010/esb-gateway-docker:latest" || service != "gateway" {
		t.Fatalf("PreferredServiceImage() = (%q,%q)", imageRef, service)
	}
}

func TestPreferredServiceImageFallsBackLexicographically(t *testing.T) {
	imageRef, service := PreferredServiceImage(map[string]string{
		"zeta":  "registry:5010/esb-zeta-docker:latest",
		"alpha": "registry:5010/esb-alpha-docker:latest",
	})
	if imageRef != "registry:5010/esb-alpha-docker:latest" || service != "alpha" {
		t.Fatalf("PreferredServiceImage() fallback = (%q,%q)", imageRef, service)
	}
}
