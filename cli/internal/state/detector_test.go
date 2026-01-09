// Where: cli/internal/state/detector_test.go
// What: Tests for Detector behavior.
// Why: Validate state detection order and dependency usage.
package state

import (
	"errors"
	"testing"
)

func TestDetectorDetect_ContextError(t *testing.T) {
	calledContainers := false
	calledArtifacts := false

	det := Detector{
		ProjectDir: "project",
		Env:        "default",
		ResolveContext: func(_, _ string) (Context, error) {
			return Context{}, errors.New("missing")
		},
		ListContainers: func(_ string) ([]ContainerInfo, error) {
			calledContainers = true
			return nil, nil
		},
		HasBuildArtifacts: func(_ string) (bool, error) {
			calledArtifacts = true
			return true, nil
		},
	}

	state, err := det.Detect()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if state != StateUninitialized {
		t.Fatalf("expected uninitialized, got %s", state)
	}
	if calledContainers || calledArtifacts {
		t.Fatalf("expected no dependency calls on context error")
	}
}

func TestDetectorDetect_RunningSkipsArtifacts(t *testing.T) {
	calledArtifacts := false

	det := Detector{
		ProjectDir: "project",
		Env:        "default",
		ResolveContext: func(_, _ string) (Context, error) {
			return Context{ComposeProject: "esb-default", OutputEnvDir: "/tmp/out"}, nil
		},
		ListContainers: func(_ string) ([]ContainerInfo, error) {
			return []ContainerInfo{{State: "running"}, {State: "exited"}}, nil
		},
		HasBuildArtifacts: func(_ string) (bool, error) {
			calledArtifacts = true
			return true, nil
		},
	}

	state, err := det.Detect()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if state != StateRunning {
		t.Fatalf("expected running, got %s", state)
	}
	if calledArtifacts {
		t.Fatalf("expected artifacts not to be checked when running")
	}
}

func TestDetectorDetect_StoppedSkipsArtifacts(t *testing.T) {
	calledArtifacts := false

	det := Detector{
		ProjectDir: "project",
		Env:        "default",
		ResolveContext: func(_, _ string) (Context, error) {
			return Context{ComposeProject: "esb-default", OutputEnvDir: "/tmp/out"}, nil
		},
		ListContainers: func(_ string) ([]ContainerInfo, error) {
			return []ContainerInfo{{State: "exited"}}, nil
		},
		HasBuildArtifacts: func(_ string) (bool, error) {
			calledArtifacts = true
			return true, nil
		},
	}

	state, err := det.Detect()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if state != StateStopped {
		t.Fatalf("expected stopped, got %s", state)
	}
	if calledArtifacts {
		t.Fatalf("expected artifacts not to be checked when stopped")
	}
}

func TestDetectorDetect_BuiltWarnsOnMissingImages(t *testing.T) {
	warned := false

	det := Detector{
		ProjectDir: "project",
		Env:        "default",
		ResolveContext: func(_, _ string) (Context, error) {
			return Context{ComposeProject: "esb-default", OutputEnvDir: "/tmp/out"}, nil
		},
		ListContainers: func(_ string) ([]ContainerInfo, error) {
			return nil, nil
		},
		HasBuildArtifacts: func(_ string) (bool, error) {
			return true, nil
		},
		HasImages: func(_ Context) (bool, error) {
			return false, nil
		},
		Warn: func(_ string) {
			warned = true
		},
	}

	state, err := det.Detect()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if state != StateBuilt {
		t.Fatalf("expected built, got %s", state)
	}
	if !warned {
		t.Fatalf("expected warning on missing images")
	}
}

func TestDetectorDetect_InitializedWhenNoArtifacts(t *testing.T) {
	det := Detector{
		ProjectDir: "project",
		Env:        "default",
		ResolveContext: func(_, _ string) (Context, error) {
			return Context{ComposeProject: "esb-default", OutputEnvDir: "/tmp/out"}, nil
		},
		ListContainers: func(_ string) ([]ContainerInfo, error) {
			return nil, nil
		},
		HasBuildArtifacts: func(_ string) (bool, error) {
			return false, nil
		},
	}

	state, err := det.Detect()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if state != StateInitialized {
		t.Fatalf("expected initialized, got %s", state)
	}
}

func TestDetectorDetect_ContainerListError(t *testing.T) {
	det := Detector{
		ProjectDir: "project",
		Env:        "default",
		ResolveContext: func(_, _ string) (Context, error) {
			return Context{ComposeProject: "esb-default", OutputEnvDir: "/tmp/out"}, nil
		},
		ListContainers: func(_ string) ([]ContainerInfo, error) {
			return nil, errors.New("boom")
		},
		HasBuildArtifacts: func(_ string) (bool, error) {
			return false, nil
		},
	}

	_, err := det.Detect()
	if err == nil {
		t.Fatalf("expected error when container listing fails")
	}
}
