package deployops

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareFunctionLayerBuildContextsPythonFlatZip(t *testing.T) {
	repoRoot := t.TempDir()
	mustWriteFile(t, filepath.Join(repoRoot, ".branding.env"), "export BRANDING_SLUG=acme\n")

	contextRoot := filepath.Join(repoRoot, "tmp-context")
	functionDir := filepath.Join(contextRoot, "functions", "lambda-echo")
	mustMkdirAll(t, functionDir)
	mustWriteFile(
		t,
		filepath.Join(functionDir, "Dockerfile"),
		"FROM esb-lambda-base:latest\n"+
			"ENV PYTHONPATH=/opt/python${PYTHONPATH:+:${PYTHONPATH}}\n"+
			"COPY --from=layer_0_zip-layer / /opt/\n",
	)
	writeZipArchive(
		t,
		filepath.Join(functionDir, "layers", "zip-layer.zip"),
		map[string]string{"lib.py": "print('ok')\n"},
	)

	contexts, err := prepareFunctionLayerBuildContexts(repoRoot, contextRoot, "lambda-echo")
	if err != nil {
		t.Fatalf("prepareFunctionLayerBuildContexts: %v", err)
	}
	path := contexts["layer_0_zip-layer"]
	if path == "" {
		t.Fatalf("expected layer context path")
	}
	expectedRoot := filepath.Join(repoRoot, ".acme", "cache", "layers")
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(expectedRoot)) {
		t.Fatalf("expected context under %s, got %s", expectedRoot, path)
	}
	if _, err := os.Stat(filepath.Join(path, "python", "lib.py")); err != nil {
		t.Fatalf("expected python-prefixed extraction: %v", err)
	}
}

func TestPrepareFunctionLayerBuildContextsKeepsPythonLayout(t *testing.T) {
	repoRoot := t.TempDir()
	contextRoot := filepath.Join(repoRoot, "tmp-context")
	functionDir := filepath.Join(contextRoot, "functions", "lambda-echo")
	mustMkdirAll(t, functionDir)
	mustWriteFile(
		t,
		filepath.Join(functionDir, "Dockerfile"),
		"FROM esb-lambda-base:latest\n"+
			"ENV PYTHONPATH=/opt/python${PYTHONPATH:+:${PYTHONPATH}}\n"+
			"COPY --from=layer_0_zip-layer / /opt/\n",
	)
	writeZipArchive(
		t,
		filepath.Join(functionDir, "layers", "zip-layer.zip"),
		map[string]string{"python/lib.py": "print('ok')\n"},
	)

	contexts, err := prepareFunctionLayerBuildContexts(repoRoot, contextRoot, "lambda-echo")
	if err != nil {
		t.Fatalf("prepareFunctionLayerBuildContexts: %v", err)
	}
	path := contexts["layer_0_zip-layer"]
	if path == "" {
		t.Fatalf("expected layer context path")
	}
	if _, err := os.Stat(filepath.Join(path, "python", "lib.py")); err != nil {
		t.Fatalf("expected nested python layout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(path, "python", "python")); err == nil {
		t.Fatal("did not expect double python nesting")
	}
}

func TestPrepareFunctionLayerBuildContextsFailsWhenLayerAliasZipMissing(t *testing.T) {
	repoRoot := t.TempDir()
	contextRoot := filepath.Join(repoRoot, "tmp-context")
	functionDir := filepath.Join(contextRoot, "functions", "lambda-echo")
	mustMkdirAll(t, functionDir)
	mustWriteFile(
		t,
		filepath.Join(functionDir, "Dockerfile"),
		"FROM esb-lambda-base:latest\n"+
			"COPY --from=layer_0_zip-layer / /opt/\n",
	)

	_, err := prepareFunctionLayerBuildContexts(repoRoot, contextRoot, "lambda-echo")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "layer archive for alias") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareFunctionLayerBuildContextsIgnoresNonLayerAlias(t *testing.T) {
	repoRoot := t.TempDir()
	contextRoot := filepath.Join(repoRoot, "tmp-context")
	functionDir := filepath.Join(contextRoot, "functions", "lambda-echo")
	mustMkdirAll(t, functionDir)
	mustWriteFile(
		t,
		filepath.Join(functionDir, "Dockerfile"),
		"FROM scratch AS custom\n"+
			"FROM esb-lambda-base:latest\n"+
			"COPY --from=custom / /opt/\n",
	)

	contexts, err := prepareFunctionLayerBuildContexts(repoRoot, contextRoot, "lambda-echo")
	if err != nil {
		t.Fatalf("prepareFunctionLayerBuildContexts: %v", err)
	}
	if len(contexts) != 0 {
		t.Fatalf("expected no layer contexts, got %+v", contexts)
	}
}

func TestPrepareFunctionLayerBuildContextsAllowsLayerAliasFromStage(t *testing.T) {
	repoRoot := t.TempDir()
	contextRoot := filepath.Join(repoRoot, "tmp-context")
	functionDir := filepath.Join(contextRoot, "functions", "lambda-echo")
	mustMkdirAll(t, functionDir)
	mustWriteFile(
		t,
		filepath.Join(functionDir, "Dockerfile"),
		"FROM scratch AS layer_0_zip-layer\n"+
			"FROM esb-lambda-base:latest\n"+
			"COPY --from=layer_0_zip-layer / /opt/\n",
	)

	contexts, err := prepareFunctionLayerBuildContexts(repoRoot, contextRoot, "lambda-echo")
	if err != nil {
		t.Fatalf("prepareFunctionLayerBuildContexts: %v", err)
	}
	if len(contexts) != 0 {
		t.Fatalf("expected no external contexts for stage-backed alias, got %+v", contexts)
	}
}

func TestResolveBrandHomeDirSanitizesAndFallsBack(t *testing.T) {
	repoRoot := t.TempDir()

	if got := resolveBrandHomeDir(repoRoot); got != ".esb" {
		t.Fatalf("expected default home dir, got %s", got)
	}

	t.Setenv("BRANDING_SLUG", "../../BAD***")
	if got := resolveBrandHomeDir(repoRoot); got != ".bad" {
		t.Fatalf("expected sanitized env slug, got %s", got)
	}

	mustWriteFile(t, filepath.Join(repoRoot, ".branding.env"), "export BRANDING_SLUG=AcMe-Prod\n")
	if got := resolveBrandHomeDir(repoRoot); got != ".acme-prod" {
		t.Fatalf("expected file slug to win, got %s", got)
	}
}

func TestIsPythonLayerLayoutRequiredHandlesEnvFormats(t *testing.T) {
	dockerfileA := "FROM base\nENV PYTHONPATH=/opt/python:${PYTHONPATH}\n"
	if !isPythonLayerLayoutRequired(dockerfileA) {
		t.Fatal("expected python layout true for key=value env")
	}

	dockerfileB := "FROM base\nenv PYTHONPATH /opt/python:${PYTHONPATH}\n"
	if !isPythonLayerLayoutRequired(dockerfileB) {
		t.Fatal("expected python layout true for key value env")
	}

	dockerfileC := "FROM base\nENV PATH=/usr/bin\n"
	if isPythonLayerLayoutRequired(dockerfileC) {
		t.Fatal("expected python layout false")
	}

	dockerfileD := "FROM base\nENV PYTHONPATH=/opt/python:${PYTHONPATH} \\\n  LANG=C.UTF-8\n"
	if !isPythonLayerLayoutRequired(dockerfileD) {
		t.Fatal("expected python layout true for line-continuation env")
	}
}

func TestParseLayerContextAliasesHandlesLineContinuation(t *testing.T) {
	dockerfile := "FROM base\nCOPY --from=layer_0_zip-layer \\\n  / /opt/\n"
	aliases := parseLayerContextAliases(dockerfile)
	if len(aliases) != 1 || aliases[0] != "layer_0_zip-layer" {
		t.Fatalf("unexpected aliases: %+v", aliases)
	}
}

func TestExtractZipToDirWithLimitRejectsOversizedEntry(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "oversized.zip")
	writeZipArchive(t, zipPath, map[string]string{
		"payload.bin": strings.Repeat("a", 16),
	})

	outputDir := filepath.Join(root, "out")
	err := extractZipToDirWithLimit(zipPath, outputDir, "", 8)
	if err == nil {
		t.Fatal("expected extraction limit error")
	}
	if !strings.Contains(err.Error(), "zip extraction exceeds limit") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(outputDir, "payload.bin")); !os.IsNotExist(statErr) {
		t.Fatalf("expected oversized partial file removed, stat err: %v", statErr)
	}
}

func TestExtractZipToDirWithLimitAllowsExactSize(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "exact.zip")
	writeZipArchive(t, zipPath, map[string]string{
		"payload.bin": strings.Repeat("a", 16),
	})

	outputDir := filepath.Join(root, "out")
	if err := extractZipToDirWithLimit(zipPath, outputDir, "", 16); err != nil {
		t.Fatalf("extractZipToDirWithLimit: %v", err)
	}
	info, err := os.Stat(filepath.Join(outputDir, "payload.bin"))
	if err != nil {
		t.Fatalf("stat payload: %v", err)
	}
	if info.Size() != 16 {
		t.Fatalf("unexpected payload size: %d", info.Size())
	}
}

func writeZipArchive(t *testing.T, path string, files map[string]string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))

	target, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer target.Close()

	writer := zip.NewWriter(target)
	for name, content := range files {
		entry, err := writer.Create(filepath.ToSlash(name))
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
}
