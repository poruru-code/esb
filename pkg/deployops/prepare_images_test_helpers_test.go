package deployops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru-code/esb/pkg/artifactcore"
)

type recordCommandRunner struct {
	commands [][]string
	hook     func([]string) error
}

func (r *recordCommandRunner) Run(cmd []string) error {
	clone := append([]string(nil), cmd...)
	r.commands = append(r.commands, clone)
	if r.hook != nil {
		return r.hook(clone)
	}
	return nil
}

func writePrepareImageFixture(t *testing.T, root, imageRef, baseRef string) string {
	t.Helper()
	artifactRoot := filepath.Join(root, "fixture")
	functionDir := filepath.Join(artifactRoot, "functions", "lambda-echo")
	configDir := filepath.Join(artifactRoot, "config")
	mustMkdirAll(t, functionDir)
	mustMkdirAll(t, configDir)
	mustWriteFile(
		t,
		filepath.Join(functionDir, "Dockerfile"),
		"FROM "+baseRef+"\nCOPY functions/lambda-echo/src/ /var/task/\n",
	)
	mustMkdirAll(t, filepath.Join(functionDir, "src"))
	mustWriteFile(t, filepath.Join(functionDir, "src", "lambda_function.py"), "def lambda_handler(event, context):\n    return {'ok': True}\n")
	mustWriteFile(
		t,
		filepath.Join(configDir, "functions.yml"),
		"functions:\n  lambda-echo:\n    image: \""+imageRef+"\"\n",
	)
	mustWriteFile(t, filepath.Join(configDir, "routing.yml"), "routes: []\n")
	mustWriteFile(t, filepath.Join(configDir, "resources.yml"), "resources: {}\n")

	manifest := artifactcore.ArtifactManifest{
		SchemaVersion: artifactcore.ArtifactSchemaVersionV1,
		Project:       "esb-e2e-docker",
		Env:           "e2e-docker",
		Mode:          "docker",
		Artifacts: []artifactcore.ArtifactEntry{
			{
				ArtifactRoot:     "fixture",
				RuntimeConfigDir: "config",
			},
		},
	}
	manifestPath := filepath.Join(root, "artifact.yml")
	if err := artifactcore.WriteArtifactManifest(manifestPath, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return manifestPath
}

func assertCommandContains(t *testing.T, cmd []string, key, value string) {
	t.Helper()
	for i := 0; i+1 < len(cmd); i++ {
		if cmd[i] == key && cmd[i+1] == value {
			return
		}
	}
	t.Fatalf("command missing %s %s: %v", key, value, cmd)
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
