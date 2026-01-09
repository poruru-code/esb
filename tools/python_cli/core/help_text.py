# Where: tools/python_cli/core/help_text.py
# What: Centralized help text constants for CLI arguments.
# Why: Avoid long lines (E501) in main.py and improve maintainability.

INIT_ENV = (
    "Environment name(s) for silent initialization (comma-separated, "
    "e.g., 'e2e-docker,e2e-containerd'). "
    "When specified, runs non-interactively with defaults."
)


NO_TEMPLATE_ERROR = (
    "\nPlease specify a template using --template or set ESB_TEMPLATE environment variable:\n"
    "  esb --template=<path/to/template.yaml> <command>\n"
    "  ESB_TEMPLATE=<path/to/template.yaml> esb <command>"
)
