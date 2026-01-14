import argparse


def parse_args():
    parser = argparse.ArgumentParser(description="E2E Test Runner (ESB CLI Wrapper)")
    parser.add_argument("--build", action="store_true", help="Rebuild images before running tests")
    parser.add_argument(
        "--cleanup", action="store_true", help="Cleanup environment after successful tests"
    )
    parser.add_argument(
        "--reset", action="store_true", help="Reset environment before running tests"
    )
    parser.add_argument("--unit", action="store_true", help="Run unit tests")
    parser.add_argument("--unit-only", action="store_true", help="Run unit tests only")
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
    parser.add_argument("--verbose", action="store_true", help="Show full logs")
    return parser.parse_args()
