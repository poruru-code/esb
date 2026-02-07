# Where: tools/tests/test_e2e_live_display.py
# What: Unit tests for E2E live display and live auto detection.
# Why: Ensure TTY gating and ANSI updates behave as expected.
from __future__ import annotations

import sys

from e2e.run_tests import resolve_live_enabled
from e2e.runner import live_display
from e2e.runner.live_display import LiveDisplay


def test_resolve_live_enabled_tty(monkeypatch) -> None:
    monkeypatch.setattr(sys.stdout, "isatty", lambda: True)
    monkeypatch.delenv("TERM", raising=False)
    monkeypatch.delenv("NO_COLOR", raising=False)
    monkeypatch.delenv("NO_EMOJI", raising=False)
    assert resolve_live_enabled(False) is True


def test_resolve_live_disabled_when_flag_set(monkeypatch) -> None:
    monkeypatch.setattr(sys.stdout, "isatty", lambda: True)
    assert resolve_live_enabled(True) is False


def test_live_display_updates(monkeypatch) -> None:
    output: list[str] = []

    def fake_write(message: str) -> None:
        output.append(message)

    monkeypatch.setattr(live_display, "write_raw", fake_write)
    display = LiveDisplay(["a", "b"], label_width=4)
    display.start()
    display.update_line("a", "[a   ] running")
    display.log_line("hello")
    rendered = "".join(output)
    assert "[a   ] waiting" in rendered
    assert "\x1b[" in rendered
    assert "hello" in rendered
