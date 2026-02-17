"""
Where: services/gateway/core/function_name.py
What: Normalize Lambda FunctionName values used by Invoke API.
Why: Keep AWS-compatible FunctionName parsing at the Gateway boundary.
"""

import re
from dataclasses import dataclass
from urllib.parse import unquote

_FULL_ARN_PATTERN = re.compile(
    r"^arn:[^:]+:lambda:[^:]+:\d{12}:function:(?P<name>[^:]+)(?::(?P<qualifier>[^:]+))?$"
)
_PARTIAL_ARN_PATTERN = re.compile(r"^\d{12}:function:(?P<name>[^:]+)(?::(?P<qualifier>[^:]+))?$")
_NAME_WITH_QUALIFIER_PATTERN = re.compile(r"^(?P<name>[^:]+):(?P<qualifier>[^:]+)$")


@dataclass(frozen=True)
class NormalizedFunctionName:
    original: str
    name: str
    qualifier: str | None = None


def normalize_invoke_function_name(function_name: str) -> NormalizedFunctionName:
    """
    Normalize FunctionName for Invoke API compatibility.

    Supported inputs:
    - function name (`my-function`)
    - function name with qualifier (`my-function:prod`)
    - full ARN (`arn:aws:lambda:region:account:function:my-function[:qualifier]`)
    - partial ARN (`account:function:my-function[:qualifier]`)
    """
    normalized = unquote(function_name).strip()
    if not normalized:
        raise ValueError("FunctionName is required")

    full_match = _FULL_ARN_PATTERN.match(normalized)
    if full_match:
        return NormalizedFunctionName(
            original=normalized,
            name=full_match.group("name"),
            qualifier=full_match.group("qualifier"),
        )

    partial_match = _PARTIAL_ARN_PATTERN.match(normalized)
    if partial_match:
        return NormalizedFunctionName(
            original=normalized,
            name=partial_match.group("name"),
            qualifier=partial_match.group("qualifier"),
        )

    name_with_qualifier = _NAME_WITH_QUALIFIER_PATTERN.match(normalized)
    if name_with_qualifier:
        return NormalizedFunctionName(
            original=normalized,
            name=name_with_qualifier.group("name"),
            qualifier=name_with_qualifier.group("qualifier"),
        )

    return NormalizedFunctionName(original=normalized, name=normalized)
