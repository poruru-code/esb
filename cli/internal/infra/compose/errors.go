// Where: cli/internal/infra/compose/errors.go
// What: Shared error definitions for compose infra.
// Why: Ensure consistent error wrapping without dynamic error creation.
package compose

import "errors"

var (
	errCommandRunnerNil = errors.New("command runner is nil")
	errRootDirRequired  = errors.New("root dir is required")
	errUnsupportedMode  = errors.New("unsupported mode")
	errDockerClientNil  = errors.New("docker client is nil")
)
