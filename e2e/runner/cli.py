import argparse


def parse_args():
    parser = argparse.ArgumentParser(description="E2E Test Runner (ESB CLI Wrapper)")
    parser.add_argument(
        "--build",
        action="store_true",
        help="Rebuild base images during docker compose up",
    )
    parser.add_argument(
        "--cleanup", action="store_true", help="Cleanup environment after successful tests"
    )
    parser.add_argument("--unit", action="store_true", help="Run unit tests")
    parser.add_argument("--unit-only", action="store_true", help="Run unit tests only")
    parser.add_argument(
        "--build-only",
        action="store_true",
        help="Run deploy phase only (skip tests)",
    )
    parser.add_argument(
        "--test-only",
        action="store_true",
        help="Run test phase only (internal use for phased execution)",
    )
    parser.add_argument(
        "--test-target", type=str, help="Specific pytest target (e.g. e2e/test_trace.py)"
    )
    parser.add_argument(
        "--profile",
        type=str,
        help="Environment to use for single target run (e.g. containerd)",
    )
    parser.add_argument("--fail-fast", action="store_true", help="Stop on first suite failure")
    parser.add_argument(
        "--parallel",
        action="store_true",
        help="Run environments in parallel (e.g., e2e-docker and e2e-containerd simultaneously)",
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="Enable verbose output",
    )
    parser.add_argument(
        "--color",
        dest="color",
        action="store_const",
        const=True,
        help="Force color output",
    )
    parser.add_argument(
        "--no-color",
        dest="color",
        action="store_const",
        const=False,
        help="Disable color output",
    )
    parser.add_argument(
        "--emoji",
        dest="emoji",
        action="store_const",
        const=True,
        help="Force emoji output",
    )
    parser.add_argument(
        "--no-emoji",
        dest="emoji",
        action="store_const",
        const=False,
        help="Disable emoji output",
    )
    parser.add_argument(
        "--no-cache",
        action="store_true",
        help="Disable build cache (pass --no-cache to deploy)",
    )
    parser.set_defaults(color=None, emoji=None)
    return parser.parse_args()
