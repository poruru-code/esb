// Where: cli/internal/version/version.go
// What: Version information retrieval.
// Why: Provide build-time version information (Git commit, state) to the CLI.
package version

import (
	"fmt"
	"runtime/debug"
)

// GetVersion returns the version information derived from build info.
// It returns "dev" if build info is not available.
// Otherwise, it returns the VCS revision, optionally appended with "(dirty)"
// if the tree was modified.
func GetVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}

	var revision string
	var modified bool

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
			// Shorten revision to 7 chars if possible
			if len(revision) > 7 {
				revision = revision[:7]
			}
		case "vcs.modified":
			if setting.Value == "true" {
				modified = true
			}
		}
	}

	if revision == "" {
		return "dev"
	}

	if modified {
		return fmt.Sprintf("%s (dirty)", revision)
	}
	return revision
}
