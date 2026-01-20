// Where: cli/internal/helpers/state_store.go
// What: Port state persistence implementation.
// Why: Store and retrieve discovered ports from a consistent location.
package helpers

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
	"github.com/poruru/edge-serverless-box/meta"
)

type portStateStore struct{}

// NewPortStateStore creates a StateStore backed by the local filesystem.
func NewPortStateStore() ports.StateStore {
	return portStateStore{}
}

func (portStateStore) Load(ctx state.Context) (map[string]int, error) {
	path, err := portsStatePath(ctx.Env)
	if err != nil {
		return nil, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]int{}, nil
		}
		return nil, err
	}
	var loaded map[string]int
	if err := json.Unmarshal(payload, &loaded); err != nil {
		return nil, err
	}
	if loaded == nil {
		return map[string]int{}, nil
	}
	return loaded, nil
}

func (portStateStore) Save(ctx state.Context, ports map[string]int) error {
	path, err := portsStatePath(ctx.Env)
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(ports, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func (portStateStore) Remove(ctx state.Context) error {
	path, err := portsStatePath(ctx.Env)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func portsStatePath(env string) (string, error) {
	home, err := resolveESBHome(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "ports.json"), nil
}

// resolveESBHome determines the base directory for project-specific data.
// Uses brand-specific HOME environment variable if set, otherwise ~/.<brand>/<env>.
func resolveESBHome(env string) (string, error) {
	override := strings.TrimSpace(envutil.GetHostEnv(constants.HostSuffixHome))
	if override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(env)
	if name == "" {
		name = "default"
	}
	homeDirName := meta.HomeDir
	if !strings.HasPrefix(homeDirName, ".") {
		homeDirName = "." + homeDirName
	}
	return filepath.Join(home, homeDirName, name), nil
}
