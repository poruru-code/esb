import time

import pytest

from services.common.models.internal import WorkerInfo


def test_worker_info_equality():
    """
    Test that WorkerInfo equality is based solely on ID (Phase 1 Requirement)
    and that it is mutable (not frozen) to allow updates.
    """
    # 1. Create two workers with same ID but different timestamps
    w1 = WorkerInfo(
        id="container-123",
        name="lambda-test-1",
        ip_address="1.2.3.4",
        created_at=100.0,
        last_used_at=100.0,
    )

    w2 = WorkerInfo(
        id="container-123",  # SAME ID
        name="lambda-test-1",  # Same name (irrelevant for equality in new spec)
        ip_address="1.2.3.4",
        created_at=100.0,
        last_used_at=200.0,  # DIFFERENT timestamp
    )

    # Check frozen status (Should be False)
    try:
        w1.last_used_at = 300.0
    except Exception as e:
        pytest.fail(f"WorkerInfo should be mutable (frozen=False): {e}")

    # Check Equality
    assert w1 == w2, "WorkerInfo with same ID should be equal"

    # Check Hashing
    assert hash(w1) == hash(w2), "WorkerInfo with same ID should have same hash"

    # Set Behavior
    s = set()
    s.add(w1)
    s.add(w2)
    assert len(s) == 1, "Set should treat same-ID workers as identical"


def test_worker_info_last_used_at():
    """Test the new last_used_at field"""
    w = WorkerInfo(id="c1", name="n1", ip_address="0.0.0.0", last_used_at=0.0)
    assert w.last_used_at == 0.0

    now = time.time()
    w.last_used_at = now
    assert w.last_used_at == now
