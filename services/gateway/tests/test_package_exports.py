"""
Where: services/gateway/tests/test_package_exports.py
What: Guard tests for package-level exports.
Why: Prevent regressions when editing package __init__.py files.
"""


def test_services_package_re_exports() -> None:
    from services.gateway.services import FunctionRegistry, RouteMatcher, SchedulerService

    assert FunctionRegistry.__name__ == "FunctionRegistry"
    assert RouteMatcher.__name__ == "RouteMatcher"
    assert SchedulerService.__name__ == "SchedulerService"
