package buildargs

import (
	"os"
	"strings"
)

type aliasPair struct {
	upper string
	lower string
}

var proxyAliasPairs = []aliasPair{
	{upper: "HTTP_PROXY", lower: "http_proxy"},
	{upper: "HTTPS_PROXY", lower: "https_proxy"},
	{upper: "NO_PROXY", lower: "no_proxy"},
}

// AppendDockerBuildArgs appends proxy-related --build-arg pairs in deterministic order.
// For each alias pair, uppercase value is preferred and lowercase is used as fallback.
func AppendDockerBuildArgs(cmd []string, env map[string]string) []string {
	for _, pair := range proxyAliasPairs {
		value := strings.TrimSpace(env[pair.upper])
		if value == "" {
			value = strings.TrimSpace(env[pair.lower])
		}
		if value == "" {
			continue
		}
		cmd = append(cmd, "--build-arg", pair.upper+"="+value)
		cmd = append(cmd, "--build-arg", pair.lower+"="+value)
	}
	return cmd
}

// EnvFromOS captures proxy aliases from process environment.
func EnvFromOS() map[string]string {
	env := make(map[string]string, len(proxyAliasPairs)*2)
	for _, pair := range proxyAliasPairs {
		env[pair.upper] = os.Getenv(pair.upper)
		env[pair.lower] = os.Getenv(pair.lower)
	}
	return env
}

// AppendDockerBuildArgsFromOS appends proxy build args using process environment aliases.
func AppendDockerBuildArgsFromOS(cmd []string) []string {
	return AppendDockerBuildArgs(cmd, EnvFromOS())
}
