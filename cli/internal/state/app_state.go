// Where: cli/internal/state/app_state.go
// What: Application-level project selection helpers.
// Why: Resolve ESB_PROJECT and recent project fallback consistently.
package state

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// PromptFunc asks the user a yes/no question and returns true on confirmation.
type PromptFunc func(message string) (bool, error)

// AppState holds project-level selection results.
type AppState struct {
	HasProjects   bool
	ActiveProject string
}

// AppStateOptions configures project selection and interaction behavior.
type AppStateOptions struct {
	ProjectEnv  string
	Projects    map[string]config.ProjectEntry
	Force       bool
	Interactive bool
	Prompt      PromptFunc
}

// ResolveAppState resolves the active project from env variables or config.
func ResolveAppState(opts AppStateOptions) (AppState, error) {
	hasProjects := len(opts.Projects) > 0
	projectEnv := strings.TrimSpace(opts.ProjectEnv)

	if projectEnv != "" {
		if _, ok := opts.Projects[projectEnv]; ok {
			return AppState{HasProjects: hasProjects, ActiveProject: projectEnv}, nil
		}
		allowed, err := confirmUnset("ESB_PROJECT", projectEnv, opts)
		if err != nil {
			return AppState{}, err
		}
		if !allowed {
			return AppState{}, fmt.Errorf("ESB_PROJECT %q not found", projectEnv)
		}
		_ = os.Unsetenv("ESB_PROJECT")
	}

	if !hasProjects {
		return AppState{HasProjects: false}, fmt.Errorf(
			"No projects registered. Run 'esb init -t <template>' to get started.",
		)
	}
	if len(opts.Projects) == 1 {
		for name := range opts.Projects {
			return AppState{HasProjects: true, ActiveProject: name}, nil
		}
	}

	latest, ok := mostRecentProject(opts.Projects)
	if !ok {
		return AppState{HasProjects: true}, fmt.Errorf(
			"No active project. Run 'esb project use <name>' first.",
		)
	}
	return AppState{HasProjects: true, ActiveProject: latest}, nil
}

func confirmUnset(envKey, value string, opts AppStateOptions) (bool, error) {
	if opts.Interactive && opts.Prompt != nil {
		message := fmt.Sprintf("%s %q not found. Unset %s?", envKey, value, envKey)
		return opts.Prompt(message)
	}
	if opts.Force {
		return true, nil
	}
	return false, fmt.Errorf("%s %q not found. Re-run with --force to auto-unset.", envKey, value)
}

func mostRecentProject(projects map[string]config.ProjectEntry) (string, bool) {
	var (
		latestName string
		latestTime time.Time
		found      bool
	)

	for name, entry := range projects {
		parsed := parseLastUsed(entry.LastUsed)
		if parsed.IsZero() {
			continue
		}
		if !found || parsed.After(latestTime) || (parsed.Equal(latestTime) && name < latestName) {
			latestName = name
			latestTime = parsed
			found = true
		}
	}
	return latestName, found
}

func parseLastUsed(value string) time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
