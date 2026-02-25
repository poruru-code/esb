package deployops

import (
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	proxybuildargs "github.com/poruru-code/esb/pkg/proxy/buildargs"
)

func buildxBuildCommandWithBuildArgs(
	tag, dockerfile, contextDir string,
	noCache bool,
	buildArgs map[string]string,
) []string {
	return buildxBuildCommandWithBuildArgsAndContexts(
		tag,
		dockerfile,
		contextDir,
		noCache,
		buildArgs,
		nil,
	)
}

func buildxBuildCommandWithBuildArgsAndContexts(
	tag, dockerfile, contextDir string,
	noCache bool,
	buildArgs map[string]string,
	buildContexts map[string]string,
) []string {
	cmd := []string{
		"docker",
		"buildx",
		"build",
		"--platform",
		"linux/amd64",
		"--load",
		"--pull",
	}
	if noCache {
		cmd = append(cmd, "--no-cache")
	}
	cmd = appendProxyBuildArgs(cmd)
	if len(buildArgs) > 0 {
		keys := make([]string, 0, len(buildArgs))
		for key := range buildArgs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := strings.TrimSpace(buildArgs[key])
			if value == "" {
				continue
			}
			cmd = append(cmd, "--build-arg", key+"="+value)
		}
	}
	if len(buildContexts) > 0 {
		keys := make([]string, 0, len(buildContexts))
		for key := range buildContexts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			contextPath := strings.TrimSpace(buildContexts[key])
			if strings.TrimSpace(key) == "" || contextPath == "" {
				continue
			}
			cmd = append(cmd, "--build-context", key+"="+contextPath)
		}
	}
	cmd = append(cmd, "--tag", tag, "--file", dockerfile, contextDir)
	return cmd
}

func buildxBuildCommand(tag, dockerfile, contextDir string, noCache bool) []string {
	return buildxBuildCommandWithBuildArgs(tag, dockerfile, contextDir, noCache, nil)
}

func appendProxyBuildArgs(cmd []string) []string {
	return proxybuildargs.AppendDockerBuildArgsFromOS(cmd)
}

func resolvePushReference(imageRef string) string {
	runtimeRegistry := strings.TrimSuffix(strings.TrimSpace(os.Getenv("CONTAINER_REGISTRY")), "/")
	hostRegistry := strings.TrimSuffix(strings.TrimSpace(os.Getenv("HOST_REGISTRY_ADDR")), "/")
	if runtimeRegistry == "" || hostRegistry == "" {
		return imageRef
	}
	prefix := runtimeRegistry + "/"
	if !strings.HasPrefix(imageRef, prefix) {
		return imageRef
	}
	suffix := strings.TrimPrefix(imageRef, prefix)
	return hostRegistry + "/" + suffix
}

func dockerImageExists(imageRef string) bool {
	cmd := exec.Command("docker", "image", "inspect", imageRef)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}
