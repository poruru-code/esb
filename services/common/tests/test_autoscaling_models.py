"""
Tests for Auto-Scaling Data Structures

TDD: RED phase - write tests first, then implement.
"""

import pytest


class TestWorkerInfo:
    """Tests for WorkerInfo dataclass"""

    def test_worker_info_creation(self):
        """WorkerInfo should be created with required fields"""
        from services.common.models.internal import WorkerInfo

        worker = WorkerInfo(
            id="container123",
            name="lambda-hello-world-abc12345",
            ip_address="172.18.0.5",
        )

        assert worker.id == "container123"
        assert worker.name == "lambda-hello-world-abc12345"
        assert worker.ip_address == "172.18.0.5"
        assert worker.port == 8080  # default
        assert worker.created_at == 0.0  # default

    def test_worker_info_with_custom_port(self):
        """WorkerInfo should accept custom port"""
        from services.common.models.internal import WorkerInfo

        worker = WorkerInfo(
            id="c1",
            name="test-worker",
            ip_address="10.0.0.1",
            port=9000,
        )

        assert worker.port == 9000

    def test_worker_info_with_created_at(self):
        """WorkerInfo should accept created_at timestamp"""
        from services.common.models.internal import WorkerInfo

        worker = WorkerInfo(
            id="c1",
            name="test-worker",
            ip_address="10.0.0.1",
            created_at=1703500000.0,
        )

        assert worker.created_at == 1703500000.0

    def test_worker_info_hashable(self):
        """WorkerInfo should be hashable for use in Set"""
        from services.common.models.internal import WorkerInfo

        worker1 = WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1")
        worker2 = WorkerInfo(id="c2", name="w2", ip_address="10.0.0.2")
        worker3 = WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1")  # same as worker1

        # Should be usable in a set
        worker_set = {worker1, worker2, worker3}

        # worker1 and worker3 have same values, but dataclass eq is by value
        # If we want identity-based, we need frozen=True or eq=False
        assert len(worker_set) == 2  # worker1 == worker3

    def test_worker_info_equality(self):
        """WorkerInfo should be equal if all fields match"""
        from services.common.models.internal import WorkerInfo

        worker1 = WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1")
        worker2 = WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1")

        assert worker1 == worker2


class TestContainerProvisionRequest:
    """Tests for ContainerProvisionRequest model"""

    def test_provision_request_minimal(self):
        """ContainerProvisionRequest with only function_name"""
        from services.common.models.internal import ContainerProvisionRequest

        req = ContainerProvisionRequest(function_name="hello-world")

        assert req.function_name == "hello-world"
        assert req.count == 1  # default
        assert req.image is None
        assert req.env == {}
        assert req.dry_run is False

    def test_provision_request_full(self):
        """ContainerProvisionRequest with all fields"""
        from services.common.models.internal import ContainerProvisionRequest

        req = ContainerProvisionRequest(
            function_name="hello-world",
            count=3,
            image="hello-world:v2",
            env={"DEBUG": "true"},
            request_id="trace-123",
            dry_run=True,
        )

        assert req.function_name == "hello-world"
        assert req.count == 3
        assert req.image == "hello-world:v2"
        assert req.env == {"DEBUG": "true"}
        assert req.request_id == "trace-123"
        assert req.dry_run is True

    def test_provision_request_count_validation(self):
        """ContainerProvisionRequest count should be between 1 and 10"""
        from pydantic import ValidationError

        from services.common.models.internal import ContainerProvisionRequest

        # Valid counts
        for count in [1, 5, 10]:
            req = ContainerProvisionRequest(function_name="test", count=count)
            assert req.count == count

        # Invalid counts
        with pytest.raises(ValidationError):
            ContainerProvisionRequest(function_name="test", count=0)

        with pytest.raises(ValidationError):
            ContainerProvisionRequest(function_name="test", count=11)


class TestContainerProvisionResponse:
    """Tests for ContainerProvisionResponse model"""

    def test_provision_response_with_workers(self):
        """ContainerProvisionResponse should contain list of WorkerInfo"""
        from services.common.models.internal import (
            ContainerProvisionResponse,
            WorkerInfo,
        )

        workers = [
            WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1"),
            WorkerInfo(id="c2", name="w2", ip_address="10.0.0.2"),
        ]

        resp = ContainerProvisionResponse(workers=workers)

        assert len(resp.workers) == 2
        assert resp.workers[0].id == "c1"
        assert resp.workers[1].id == "c2"


class TestHeartbeatRequest:
    """Tests for HeartbeatRequest model"""

    def test_heartbeat_request(self):
        """HeartbeatRequest should contain function_name and container_names"""
        from services.common.models.internal import HeartbeatRequest

        req = HeartbeatRequest(
            function_name="hello-world",
            container_names=["c1", "c2", "c3"],
        )

        assert req.function_name == "hello-world"
        assert req.container_names == ["c1", "c2", "c3"]

    def test_heartbeat_request_empty_names(self):
        """HeartbeatRequest with empty container_names list"""
        from services.common.models.internal import HeartbeatRequest

        req = HeartbeatRequest(
            function_name="hello-world",
            container_names=[],
        )

        assert req.container_names == []
