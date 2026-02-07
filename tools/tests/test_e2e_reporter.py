# Where: tools/tests/test_e2e_reporter.py
# What: Unit tests for E2E reporter formatting.
# Why: Ensure compact output stays readable and emoji toggles are respected.
from e2e.runner.events import EVENT_PHASE_END, PHASE_TEST, STATUS_PASSED, Event
from e2e.runner.ui import PlainReporter


def test_plain_reporter_compact_with_emoji(capsys) -> None:
    reporter = PlainReporter(
        verbose=False,
        env_label_width=10,
        color=False,
        emoji=True,
    )
    reporter.emit(
        Event(
            EVENT_PHASE_END,
            env="e2e",
            phase=PHASE_TEST,
            data={"status": STATUS_PASSED, "duration": 1.23},
        )
    )
    out = capsys.readouterr().out.strip()
    assert "[e2e       ]" in out
    assert "✅" in out
    assert "test" in out
    assert "ok" in out


def test_plain_reporter_no_emoji(capsys) -> None:
    reporter = PlainReporter(
        verbose=False,
        env_label_width=8,
        color=False,
        emoji=False,
    )
    reporter.emit(
        Event(
            EVENT_PHASE_END,
            env="dev",
            phase=PHASE_TEST,
            data={"status": STATUS_PASSED, "duration": 0.4},
        )
    )
    out = capsys.readouterr().out.strip()
    assert "✅" not in out
    assert "[dev     ]" in out
    assert "ok" in out
