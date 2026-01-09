// Where: cli/internal/state/detector.go
// What: State detector orchestration.
// Why: Compose context resolution, container checks, and artifacts into a state.
package state

import "fmt"

type Detector struct {
	ProjectDir string
	Env        string

	ResolveContext    func(projectDir, env string) (Context, error)
	ListContainers    func(composeProject string) ([]ContainerInfo, error)
	HasBuildArtifacts func(outputEnvDir string) (bool, error)
	HasImages         func(ctx Context) (bool, error)
	Warn              func(message string)
}

func (d Detector) Detect() (State, error) {
	resolver := d.ResolveContext
	if resolver == nil {
		resolver = ResolveContext
	}

	ctx, err := resolver(d.ProjectDir, d.Env)
	if err != nil {
		return StateUninitialized, nil
	}

	if d.ListContainers == nil {
		return StateInitialized, fmt.Errorf("container lister not configured")
	}

	containers, err := d.ListContainers(ctx.ComposeProject)
	if err != nil {
		return StateInitialized, err
	}

	hasArtifacts := false
	if len(containers) == 0 {
		if d.HasBuildArtifacts == nil {
			return StateInitialized, fmt.Errorf("artifact checker not configured")
		}
		hasArtifacts, err = d.HasBuildArtifacts(ctx.OutputEnvDir)
		if err != nil {
			return StateInitialized, err
		}
	}

	state := DeriveState(true, containers, hasArtifacts)
	if state == StateBuilt && d.HasImages != nil {
		hasImages, err := d.HasImages(ctx)
		if err != nil {
			return state, err
		}
		if !hasImages {
			warn := d.Warn
			if warn == nil {
				warn = func(string) {}
			}
			warn("images missing; run `esb build`")
		}
	}

	return state, nil
}
