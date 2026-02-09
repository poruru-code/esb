// Where: cli/internal/infra/build/stage_java_env_test.go
// What: Tests for Java Maven proxy contract in deploy-time build path.
// Why: Keep Go implementation aligned with Python via shared case vectors.
package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mavenProxyCase struct {
	Name                   string            `json:"name"`
	Env                    map[string]string `json:"env"`
	ExpectedXML            string            `json:"expected_xml"`
	ExpectedErrorSubstring string            `json:"expected_error_substring"`
}

func TestAppendJavaBuildEnvArgsClearsProxyInputs(t *testing.T) {
	args := appendJavaBuildEnvArgs(nil)
	got := envAssignments(args)

	for _, key := range []string{
		"HTTP_PROXY",
		"http_proxy",
		"HTTPS_PROXY",
		"https_proxy",
		"NO_PROXY",
		"no_proxy",
		"MAVEN_OPTS",
		"JAVA_TOOL_OPTIONS",
	} {
		value, ok := got[key]
		if !ok {
			t.Fatalf("missing %s in args: %#v", key, got)
		}
		if value != "" {
			t.Fatalf("expected %s to be empty, got %q", key, value)
		}
	}
}

func TestJavaRuntimeMavenBuildLineAlwaysUsesSettingsFile(t *testing.T) {
	t.Parallel()

	line := javaRuntimeMavenBuildLine()
	if !strings.Contains(
		line,
		"mvn -s "+containerM2SettingsPath+" -q -Dmaven.artifact.threads=1 -DskipTests",
	) {
		t.Fatalf("expected settings-enabled mvn command in %q", line)
	}
	if strings.Contains(line, "if [ -f") {
		t.Fatalf("unexpected fallback condition in %q", line)
	}
	if strings.Contains(line, "else mvn") {
		t.Fatalf("unexpected fallback mvn command in %q", line)
	}
}

func TestRenderMavenSettingsXMLMatchesSharedCases(t *testing.T) {
	for _, tc := range loadMavenProxyCases(t) {
		if tc.ExpectedXML == "" {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			applyCaseEnv(t, tc.Env)
			got, err := renderMavenSettingsXML()
			if err != nil {
				t.Fatalf("renderMavenSettingsXML returned error: %v", err)
			}
			if got != tc.ExpectedXML {
				t.Fatalf("rendered XML mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tc.ExpectedXML)
			}
		})
	}
}

func TestRenderMavenSettingsXMLRejectsInvalidSharedCases(t *testing.T) {
	for _, tc := range loadMavenProxyCases(t) {
		if tc.ExpectedErrorSubstring == "" {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			applyCaseEnv(t, tc.Env)
			_, err := renderMavenSettingsXML()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.ExpectedErrorSubstring)
			}
			if !strings.Contains(err.Error(), tc.ExpectedErrorSubstring) {
				t.Fatalf("expected error containing %q, got %v", tc.ExpectedErrorSubstring, err)
			}
		})
	}
}

func TestWriteTempMavenSettingsFileAlwaysCreatesExpectedSettings(t *testing.T) {
	cases := loadMavenProxyCases(t)
	if len(cases) == 0 || cases[0].ExpectedXML == "" {
		t.Fatalf("missing expected XML case in shared vectors")
	}
	applyCaseEnv(t, cases[0].Env)

	path, err := writeTempMavenSettingsFile()
	if err != nil {
		t.Fatalf("writeTempMavenSettingsFile returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(path)
	})

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read generated settings: %v", err)
	}
	if string(content) != cases[0].ExpectedXML {
		t.Fatalf("generated settings mismatch\n--- got ---\n%s\n--- want ---\n%s", string(content), cases[0].ExpectedXML)
	}
}

func TestJavaRuntimeBuildImageUsesAWSBuildImage(t *testing.T) {
	t.Parallel()

	want := "public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7"
	if javaRuntimeBuildImage != want {
		t.Fatalf("javaRuntimeBuildImage=%q, want %q", javaRuntimeBuildImage, want)
	}
	if strings.Contains(javaRuntimeBuildImage, ":latest") {
		t.Fatalf("javaRuntimeBuildImage must be digest-pinned, got %q", javaRuntimeBuildImage)
	}
	if !strings.Contains(javaRuntimeBuildImage, "@sha256:") {
		t.Fatalf("javaRuntimeBuildImage must include digest, got %q", javaRuntimeBuildImage)
	}
}

func loadMavenProxyCases(t *testing.T) []mavenProxyCase {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "runtime", "java", "testdata", "maven_proxy_cases.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read shared proxy cases: %v", err)
	}
	var cases []mavenProxyCase
	if err := json.Unmarshal(payload, &cases); err != nil {
		t.Fatalf("parse shared proxy cases: %v", err)
	}
	if len(cases) == 0 {
		t.Fatalf("shared proxy cases are empty")
	}
	return cases
}

func applyCaseEnv(t *testing.T, values map[string]string) {
	t.Helper()
	for _, key := range []string{
		"HTTP_PROXY",
		"http_proxy",
		"HTTPS_PROXY",
		"https_proxy",
		"NO_PROXY",
		"no_proxy",
		"MAVEN_OPTS",
		"JAVA_TOOL_OPTIONS",
	} {
		t.Setenv(key, "")
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
}

func envAssignments(args []string) map[string]string {
	assignments := make(map[string]string)
	for idx := 0; idx < len(args)-1; idx++ {
		if args[idx] != "-e" {
			continue
		}
		key, value, ok := strings.Cut(args[idx+1], "=")
		if !ok {
			continue
		}
		assignments[key] = value
	}
	return assignments
}
