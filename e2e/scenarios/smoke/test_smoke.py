"""
Where: e2e/scenarios/smoke/test_smoke.py
What: Runtime-agnostic smoke tests for connectivity actions.
Why: Keep smoke verification identical across languages in a single place.
"""

import pytest

from e2e.scenarios.smoke.runtime_helpers import (
    run_chain_invoke,
    run_dynamodb_put,
    run_echo,
    run_s3_put,
)
from e2e.scenarios.smoke.runtime_matrix import RUNTIMES


def _runtime_ids():
    return [runtime["id"] for runtime in RUNTIMES]


@pytest.mark.parametrize("runtime", RUNTIMES, ids=_runtime_ids())
def test_runtime_echo(runtime, auth_token):
    run_echo(runtime, auth_token)


@pytest.mark.parametrize("runtime", RUNTIMES, ids=_runtime_ids())
def test_runtime_dynamodb_put(runtime, auth_token):
    run_dynamodb_put(runtime, auth_token)


@pytest.mark.parametrize("runtime", RUNTIMES, ids=_runtime_ids())
def test_runtime_s3_put(runtime, auth_token):
    run_s3_put(runtime, auth_token)


@pytest.mark.parametrize("runtime", RUNTIMES, ids=_runtime_ids())
def test_runtime_chain_invoke(runtime, auth_token):
    run_chain_invoke(runtime, auth_token)
