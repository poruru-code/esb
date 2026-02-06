"""
Where: e2e/scenarios/smoke/runtime_matrix.py
What: Runtime matrix for common smoke tests.
Why: Ensure identical smoke coverage across languages.
"""

RUNTIMES = [
    {"id": "python", "path": "/api/connectivity/python"},
    {"id": "java", "path": "/api/connectivity/java"},
]

RUNTIME_BY_ID = {runtime["id"]: runtime for runtime in RUNTIMES}
