package buildargs

import (
	"slices"
	"testing"
)

func assertContainsPair(t *testing.T, cmd []string, key, expected string) {
	t.Helper()
	for i := 0; i+1 < len(cmd); i++ {
		if cmd[i] == key && cmd[i+1] == expected {
			return
		}
	}
	t.Fatalf("expected command to contain %s %s, got: %v", key, expected, cmd)
}

func TestAppendDockerBuildArgsPrefersUppercaseAlias(t *testing.T) {
	cmd := AppendDockerBuildArgs([]string{"docker", "buildx", "build"}, map[string]string{
		"HTTP_PROXY":  "http://upper-http.example:8080",
		"http_proxy":  "http://lower-http.example:8080",
		"HTTPS_PROXY": "",
		"https_proxy": "http://lower-https.example:8443",
		"NO_PROXY":    "localhost,127.0.0.1",
		"no_proxy":    "ignored.local",
	})

	assertContainsPair(t, cmd, "--build-arg", "HTTP_PROXY=http://upper-http.example:8080")
	assertContainsPair(t, cmd, "--build-arg", "http_proxy=http://upper-http.example:8080")
	assertContainsPair(t, cmd, "--build-arg", "HTTPS_PROXY=http://lower-https.example:8443")
	assertContainsPair(t, cmd, "--build-arg", "https_proxy=http://lower-https.example:8443")
	assertContainsPair(t, cmd, "--build-arg", "NO_PROXY=localhost,127.0.0.1")
	assertContainsPair(t, cmd, "--build-arg", "no_proxy=localhost,127.0.0.1")
}

func TestAppendDockerBuildArgsSkipsUnsetAliases(t *testing.T) {
	cmd := AppendDockerBuildArgs([]string{"docker", "buildx", "build"}, map[string]string{})
	if !slices.Equal(cmd, []string{"docker", "buildx", "build"}) {
		t.Fatalf("unexpected command: %v", cmd)
	}
}

func TestAppendDockerBuildArgsFromOSReadsAliases(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("http_proxy", "http://proxy.example:8080")
	t.Setenv("HTTPS_PROXY", "HTTPS://secure-proxy.example:8443")
	t.Setenv("https_proxy", "")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1")
	t.Setenv("no_proxy", "")

	cmd := AppendDockerBuildArgsFromOS([]string{"docker", "buildx", "build"})
	assertContainsPair(t, cmd, "--build-arg", "HTTP_PROXY=http://proxy.example:8080")
	assertContainsPair(t, cmd, "--build-arg", "http_proxy=http://proxy.example:8080")
	assertContainsPair(t, cmd, "--build-arg", "HTTPS_PROXY=HTTPS://secure-proxy.example:8443")
	assertContainsPair(t, cmd, "--build-arg", "https_proxy=HTTPS://secure-proxy.example:8443")
	assertContainsPair(t, cmd, "--build-arg", "NO_PROXY=localhost,127.0.0.1")
	assertContainsPair(t, cmd, "--build-arg", "no_proxy=localhost,127.0.0.1")
}
