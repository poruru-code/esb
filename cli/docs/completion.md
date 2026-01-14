# `esb completion` Command

## Overview

The `esb completion` command generates shell completion scripts for Bash, Zsh, and Fish. These scripts enable tab completion for subcommands, flags, and dynamic values like environment names, project names, and services.

## Usage

```bash
esb completion [shell]
```

### Subcommands

| Command | Description |
|---------|-------------|
| `bash` | Generate Bash completion script. |
| `zsh` | Generate Zsh completion script. |
| `fish` | Generate Fish completion script. |

## Implementation Details

The command logic is implemented in `cli/internal/app/completion.go`.

### Dynamic Completion Logic

The generated scripts invoke the hidden `esb __complete` command to fetch dynamic suggestions at runtime.

- `__complete env`: Lists available environments from `generator.yml`.
- `__complete project`: Lists registered projects from global config.
- `__complete service`: Lists Docker services (for `logs` and `env var`).

### Bash Implementation
- Uses `compgen` and case statements to handle context.
- Helper functions `_esb_find_index` and `_esb_has_positional_after` track cursor position relative to commands.

### Zsh Implementation
- Uses `compdef` and `_values` for richer descriptions.
- Delegates to `esb __complete` for dynamic lists.

### Fish Implementation
- Uses `complete -c esb ... -a '(esb __complete ...)'`.
- Defines conditions to ensure completion only triggers in the correct context (e.g., after `use` or `remove`).
