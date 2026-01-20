// Where: cli/internal/workflows/project_helpers.go
// What: Project selection and sorting helpers.
// Why: Reuse project ordering logic across workflows and app wrappers.
package workflows

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// RecentProject holds project metadata for recent ordering.
type RecentProject struct {
	Name   string
	Entry  config.ProjectEntry
	UsedAt time.Time
}

// SortProjectsByRecent returns projects ordered by most recent usage.
func SortProjectsByRecent(cfg config.GlobalConfig) []RecentProject {
	projects := make([]RecentProject, 0, len(cfg.Projects))
	for name, entry := range cfg.Projects {
		projects = append(projects, RecentProject{
			Name:   name,
			Entry:  entry,
			UsedAt: ParseLastUsed(entry.LastUsed),
		})
	}

	sort.Slice(projects, func(i, j int) bool {
		if projects[i].UsedAt.Equal(projects[j].UsedAt) {
			return projects[i].Name < projects[j].Name
		}
		return projects[i].UsedAt.After(projects[j].UsedAt)
	})
	return projects
}

// ParseLastUsed parses an RFC3339 timestamp string into a time.Time.
func ParseLastUsed(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// SelectProject resolves a project selector (name or 1-indexed recent index).
func SelectProject(cfg config.GlobalConfig, selector string) (string, error) {
	if len(cfg.Projects) == 0 {
		return "", fmt.Errorf("no projects registered")
	}

	if _, ok := cfg.Projects[selector]; ok {
		return selector, nil
	}

	if index, err := strconv.Atoi(selector); err == nil {
		if index <= 0 {
			return "", fmt.Errorf("invalid project index")
		}
		list := SortProjectsByRecent(cfg)
		if index > len(list) {
			return "", fmt.Errorf("project index out of range")
		}
		return list[index-1].Name, nil
	}

	return "", fmt.Errorf("project not found")
}
