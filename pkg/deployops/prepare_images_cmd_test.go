package deployops

import (
	"strings"
	"testing"
)

func TestBuildxBuildCommandPropagatesProxyEnv(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://upper-http.example:8080")
	t.Setenv("http_proxy", "http://lower-http.example:8080")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "http://lower-https.example:8443")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1,registry")
	t.Setenv("no_proxy", "lower.local")

	cmd := buildxBuildCommand("example:latest", "/tmp/Dockerfile", "/tmp/context", false)

	assertCommandContains(t, cmd, "--build-arg", "HTTP_PROXY=http://upper-http.example:8080")
	assertCommandContains(t, cmd, "--build-arg", "http_proxy=http://upper-http.example:8080")
	assertCommandContains(t, cmd, "--build-arg", "HTTPS_PROXY=http://lower-https.example:8443")
	assertCommandContains(t, cmd, "--build-arg", "https_proxy=http://lower-https.example:8443")
	assertCommandContains(t, cmd, "--build-arg", "NO_PROXY=localhost,127.0.0.1,registry")
	assertCommandContains(t, cmd, "--build-arg", "no_proxy=localhost,127.0.0.1,registry")
}

func TestBuildxBuildCommandSkipsProxyBuildArgsWhenUnset(t *testing.T) {
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("http_proxy", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	cmd := buildxBuildCommand("example:latest", "/tmp/Dockerfile", "/tmp/context", false)
	joined := strings.Join(cmd, " ")

	for _, token := range []string{
		"HTTP_PROXY=",
		"http_proxy=",
		"HTTPS_PROXY=",
		"https_proxy=",
		"NO_PROXY=",
		"no_proxy=",
	} {
		if strings.Contains(joined, token) {
			t.Fatalf("unexpected proxy build arg %q in command: %v", token, cmd)
		}
	}
}
