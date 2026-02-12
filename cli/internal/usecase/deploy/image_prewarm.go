// Where: cli/internal/usecase/deploy/image_prewarm.go
// What: Deploy-time image prewarm workflow for image-based Lambda functions.
// Why: Fail fast on missing/unauthorized external images before runtime invocation.
package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
)

const (
	imagePullFailedCode     = "IMAGE_PULL_FAILED"
	imageAuthFailedCode     = "IMAGE_AUTH_FAILED"
	imagePushFailedCode     = "IMAGE_PUSH_FAILED"
	imageDigestMismatchCode = "IMAGE_DIGEST_MISMATCH"
)

// NormalizeImagePrewarmMode normalizes deploy image prewarm mode.
func NormalizeImagePrewarmMode(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "all", nil
	}
	switch normalized {
	case "off", "all":
		return normalized, nil
	default:
		return "", fmt.Errorf("deploy: invalid --image-prewarm value %q (use off|all)", value)
	}
}

type imageImportManifest struct {
	Version    string             `json:"version"`
	PushTarget string             `json:"push_target"`
	Images     []imageImportEntry `json:"images"`
}

type imageImportEntry struct {
	FunctionName string `json:"function_name"`
	ImageSource  string `json:"image_source"`
	ImageRef     string `json:"image_ref"`
}

func runImagePrewarm(
	ctx context.Context,
	runner compose.CommandRunner,
	user ui.UserInterface,
	manifestPath string,
	verbose bool,
) error {
	if runner == nil {
		return errComposeRunnerNotConfigured
	}

	manifest, exists, err := loadImageImportManifest(manifestPath)
	if err != nil {
		return err
	}
	if !exists || len(manifest.Images) == 0 {
		return nil
	}
	if user != nil {
		user.Info(fmt.Sprintf("Image prewarm: %d image(s)", len(manifest.Images)))
	}

	for _, entry := range manifest.Images {
		source := strings.TrimSpace(entry.ImageSource)
		target := strings.TrimSpace(entry.ImageRef)
		if source == "" || target == "" {
			continue
		}
		pushTarget := resolveHostPushTarget(target, manifest.PushTarget)
		if verbose && user != nil {
			user.Info(fmt.Sprintf("Prewarm image: %s -> %s", source, pushTarget))
		}

		if output, err := runner.RunOutput(ctx, "", "docker", "pull", source); err != nil {
			return classifyImageSyncError(imagePullFailedCode, source, output, err)
		}
		if output, err := runner.RunOutput(ctx, "", "docker", "tag", source, pushTarget); err != nil {
			return classifyImageSyncError(imageDigestMismatchCode, pushTarget, output, err)
		}
		if output, err := runner.RunOutput(ctx, "", "docker", "push", pushTarget); err != nil {
			return classifyImageSyncError(imagePushFailedCode, pushTarget, output, err)
		}
	}

	return nil
}

func resolveHostPushTarget(imageRef, pushTarget string) string {
	target := strings.TrimSpace(imageRef)
	if target == "" {
		return ""
	}
	hostTarget := strings.TrimSpace(pushTarget)
	if hostTarget == "" {
		return target
	}
	hostTarget = strings.TrimPrefix(hostTarget, "http://")
	hostTarget = strings.TrimPrefix(hostTarget, "https://")
	hostTarget = strings.TrimSuffix(hostTarget, "/")

	slash := strings.Index(target, "/")
	if slash <= 0 {
		return target
	}
	return hostTarget + target[slash:]
}

func loadImageImportManifest(path string) (imageImportManifest, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return imageImportManifest{}, false, nil
		}
		return imageImportManifest{}, false, err
	}
	var manifest imageImportManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return imageImportManifest{}, false, err
	}
	return manifest, true, nil
}

func classifyImageSyncError(code, image string, output []byte, err error) error {
	message := strings.ToLower(string(output))
	if strings.Contains(message, "unauthorized") ||
		strings.Contains(message, "authentication required") ||
		strings.Contains(message, "denied") {
		code = imageAuthFailedCode
	}
	return fmt.Errorf("%s: image=%s: %w", code, image, err)
}
