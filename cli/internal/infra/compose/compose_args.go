// Where: cli/internal/infra/compose/compose_args.go
// What: Shared helpers for building docker compose arguments.
// Why: Keep compose argument construction consistent across commands.
package compose

import "strings"

func buildComposeArgs(rootDir, mode, target, project string, extraFiles []string) ([]string, error) {
	if strings.TrimSpace(rootDir) == "" {
		return nil, errRootDirRequired
	}

	files, err := ResolveComposeFiles(rootDir, mode, target)
	if err != nil {
		return nil, err
	}

	args := []string{"compose"}
	if strings.TrimSpace(project) != "" {
		args = append(args, "-p", project)
	}
	for _, file := range files {
		args = append(args, "-f", file)
	}
	for _, file := range extraFiles {
		if strings.TrimSpace(file) == "" {
			continue
		}
		args = append(args, "-f", file)
	}

	return args, nil
}
