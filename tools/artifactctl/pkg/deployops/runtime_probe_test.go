package deployops

import (
	"reflect"
	"testing"

	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

func TestParseServiceImages(t *testing.T) {
	raw := "gateway registry:5010/esb-gateway-docker:latest\nagent registry:5010/esb-agent-docker:latest\n"
	got := parseServiceImages(raw)
	want := map[string]string{
		"gateway": "registry:5010/esb-gateway-docker:latest",
		"agent":   "registry:5010/esb-agent-docker:latest",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseServiceImages() = %#v, want %#v", got, want)
	}
}

func TestInferRuntimeModeFromServiceImages(t *testing.T) {
	if got := artifactcore.InferRuntimeModeFromServiceImages(map[string]string{"gateway": "registry:5010/esb-gateway-docker:latest"}); got != "docker" {
		t.Fatalf("InferRuntimeModeFromServiceImages() docker = %q", got)
	}
	if got := artifactcore.InferRuntimeModeFromServiceImages(map[string]string{"gateway": "registry:5010/esb-gateway-containerd:latest"}); got != "containerd" {
		t.Fatalf("InferRuntimeModeFromServiceImages() containerd = %q", got)
	}
	if got := artifactcore.InferRuntimeModeFromServiceImages(map[string]string{"gateway": "public.ecr.aws/lambda/python:3.12"}); got != "" {
		t.Fatalf("InferRuntimeModeFromServiceImages() unknown = %q", got)
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
			if got := artifactcore.ParseRuntimeImageTag(tc.image); got != tc.want {
				t.Fatalf("ParseRuntimeImageTag(%q) = %q, want %q", tc.image, got, tc.want)
			}
		})
	}
}
