package artifactcore

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

func applyWithWarnings(req ApplyInput) ([]string, error) {
	manifest, err := ReadArtifactManifest(req.ArtifactPath)
	if err != nil {
		return nil, err
	}
	warnings, err := validateRuntimeMetadata(manifest, req.ArtifactPath, req.Strict)
	if err != nil {
		return nil, err
	}
	if err := validateRequiredSecrets(manifest, req.SecretEnvPath); err != nil {
		return nil, err
	}
	if err := mergeWithManifest(req.ArtifactPath, req.OutputDir, manifest); err != nil {
		return nil, err
	}
	return warnings, nil
}

func validateRequiredSecrets(manifest ArtifactManifest, secretEnvPath string) error {
	required := collectRequiredSecretEnv(manifest)
	if len(required) == 0 {
		return nil
	}
	if strings.TrimSpace(secretEnvPath) == "" {
		return fmt.Errorf("%w: required_secret_env is defined: %s", ErrSecretEnvFileRequired, strings.Join(required, ", "))
	}
	provided, err := readEnvKeys(secretEnvPath)
	if err != nil {
		return err
	}
	missing := make([]string, 0)
	for _, key := range required {
		if _, ok := provided[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return MissingSecretKeysError{Keys: missing}
	}
	return nil
}

func collectRequiredSecretEnv(manifest ArtifactManifest) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, entry := range manifest.Artifacts {
		for _, key := range entry.RequiredSecretEnv {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			out = append(out, trimmed)
		}
	}
	sort.Strings(out)
	return out
}

func readEnvKeys(path string) (map[string]struct{}, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("open secret env file: %w", MissingReferencedPathError{Path: path})
		}
		return nil, fmt.Errorf("open secret env file: %w", err)
	}
	defer file.Close()

	keys := map[string]struct{}{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, _, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keys[key] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read secret env file: %w", err)
	}
	return keys, nil
}
