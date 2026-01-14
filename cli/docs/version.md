# `esb version` Command

## Overview

The `esb version` command displays the current version of the CLI.

## Usage

```bash
esb version
```

## Implementation Details

- **Location**: `cli/internal/app/app.go` (`runVersion`).
- **Logic**: Prints the string returned by `version.GetVersion()`.
- **Build Time**: The version is typically injected via linker flags (`-ldflags`) during the build process (handled in `cli/version/version.go` or similar).
