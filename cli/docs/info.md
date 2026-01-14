# `esb info` Command (Default)

## Overview

The `esb info` command displays a summary of the current system state, including CLI version, configuration paths, active project details, and the runtime state of the environment.

**Note**: This command is implicitly executed when `esb` is run without any arguments.

## Usage

```bash
esb
# or
esb info
```

## Implementation Details

The command logic is implemented in `cli/internal/app/info.go`.

### Information Displayed

1. **Version**: CLI version.
2. **Config**: Global configuration path.
3. **Project**:
   - Name and Root Directory.
   - Generator Config Path (`generator.yml`).
   - SAM Template Path.
   - Output Directory.
4. **Environment**:
   - Active Environment Name and Mode (e.g., `local (docker)`).
   - **State**: Derived via `StateDetector` (e.g., `running`, `stopped`, `built`).
   - Compose Project Name (`esb-local`).

### Logic Flow

1. **Global Config Load**: Validates `~/.config/esb/config.yaml`.
2. **Project Resolution**: Identifies the active project.
3. **State Detection**:
   - Uses `StateDetector` to query Docker and file system.
   - Reports "uninitialized" if context is missing, or real-time status if configured.
