// Where: cli/internal/compose/project_files.go
// What: Resolve compose config files from running Docker Compose projects.
// Why: Align CLI operations with the exact compose files used to start a project.
package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

// FilesResult captures resolved compose config files for a project.
type FilesResult struct {
	Files    []string
	Missing  []string
	SetCount int
}

// ResolveComposeFilesFromProject returns the compose config files used to start
// the specified project. If multiple sets are detected, the most common set
// among running containers is selected.
func ResolveComposeFilesFromProject(ctx context.Context, client DockerClient, project string) (FilesResult, error) {
	trimmedProject := strings.TrimSpace(project)
	if trimmedProject == "" {
		return FilesResult{}, nil
	}
	if client == nil {
		return FilesResult{}, fmt.Errorf("docker client is nil")
	}

	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("%s=%s", ComposeProjectLabel, trimmedProject))
	containers, err := client.ContainerList(ctx, container.ListOptions{All: true, Filters: filterArgs})
	if err != nil {
		return FilesResult{}, err
	}
	if len(containers) == 0 {
		return FilesResult{}, nil
	}

	candidates := containers
	running := make([]container.Summary, 0, len(containers))
	for _, ctr := range containers {
		if strings.EqualFold(ctr.State, "running") {
			running = append(running, ctr)
		}
	}
	if len(running) > 0 {
		candidates = running
	}

	type fileSet struct {
		files []string
		count int
	}
	sets := map[string]*fileSet{}
	order := []string{}
	for _, ctr := range candidates {
		labels := ctr.Labels
		if labels == nil {
			continue
		}
		raw := strings.TrimSpace(labels[ComposeConfigFilesLabel])
		if raw == "" {
			continue
		}
		workingDir := strings.TrimSpace(labels[ComposeWorkingDirLabel])
		files := normalizeComposeFilePaths(parseComposeFiles(raw), workingDir)
		if len(files) == 0 {
			continue
		}
		key := strings.Join(files, "\x1f")
		entry, ok := sets[key]
		if !ok {
			entry = &fileSet{files: files}
			sets[key] = entry
			order = append(order, key)
		}
		entry.count++
	}
	if len(sets) == 0 {
		return FilesResult{}, nil
	}

	bestKey := order[0]
	bestCount := sets[bestKey].count
	for _, key := range order[1:] {
		if sets[key].count > bestCount {
			bestKey = key
			bestCount = sets[key].count
		}
	}

	files := sets[bestKey].files
	existing := make([]string, 0, len(files))
	missing := []string{}
	for _, file := range files {
		if _, err := os.Stat(file); err != nil {
			missing = append(missing, file)
			continue
		}
		existing = append(existing, file)
	}
	return FilesResult{
		Files:    existing,
		Missing:  missing,
		SetCount: len(sets),
	}, nil
}

func parseComposeFiles(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func normalizeComposeFilePaths(files []string, workingDir string) []string {
	out := make([]string, 0, len(files))
	seen := map[string]struct{}{}
	for _, file := range files {
		path := strings.TrimSpace(file)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) && strings.TrimSpace(workingDir) != "" {
			path = filepath.Join(workingDir, path)
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		out = append(out, path)
		seen[path] = struct{}{}
	}
	return out
}
