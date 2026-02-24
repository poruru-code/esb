package deployops

import (
	"fmt"
	"strings"

	"github.com/poruru-code/esb/pkg/deployops/mavenshim"
)

func ensureMavenShimImage(
	baseRef string,
	noCache bool,
	runner CommandRunner,
	resolvedMavenShimImages map[string]string,
) (string, error) {
	baseRef = strings.TrimSpace(baseRef)
	if baseRef == "" {
		return "", fmt.Errorf("maven base image reference is empty")
	}
	if shimRef, ok := resolvedMavenShimImages[baseRef]; ok {
		return shimRef, nil
	}

	result, err := mavenshim.EnsureImage(mavenshim.EnsureInput{
		BaseImage:    baseRef,
		HostRegistry: resolveHostFunctionRegistry(),
		NoCache:      noCache,
		Runner:       mavenShimRunnerAdapter{runner: runner},
		ImageExists:  dockerImageExistsFunc,
	})
	if err != nil {
		return "", err
	}
	shimRef := result.ShimImage

	resolvedMavenShimImages[baseRef] = shimRef
	return shimRef, nil
}

type mavenShimRunnerAdapter struct {
	runner CommandRunner
}

func (a mavenShimRunnerAdapter) Run(cmd []string) error {
	if a.runner == nil {
		return fmt.Errorf("command runner is nil")
	}
	return a.runner.Run(cmd)
}
