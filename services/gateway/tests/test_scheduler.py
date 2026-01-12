from unittest.mock import AsyncMock

import pytest
from apscheduler.triggers.interval import IntervalTrigger

from services.gateway.services.scheduler import AWSCronTrigger, SchedulerService


@pytest.fixture
def mock_invoker():
    return AsyncMock()


@pytest.fixture
def scheduler_service(mock_invoker):
    return SchedulerService(mock_invoker)


def test_parse_cron_expression(scheduler_service):
    # cron(Minutes Hours Day-of-month Month Day-of-week Year)
    # AWS: cron(0 12 * * ? *) -> Everyday at 12:00 UTC
    trigger = scheduler_service._parse_expression("cron(0 12 * * ? *)")
    assert isinstance(trigger, AWSCronTrigger)

    # Check by string representation
    trigger_str = str(trigger)
    assert "aws_cron(0 12 * * ? *)" in trigger_str


def test_parse_cron_complex(scheduler_service):
    """Verify that complex AWS cron syntax (L, #) is accepted."""
    # Last day of month
    trigger = scheduler_service._parse_expression("cron(0 10 L * ? *)")
    assert isinstance(trigger, AWSCronTrigger)

    # Last Friday of month
    trigger = scheduler_service._parse_expression("cron(0 10 ? * 6L *)")
    assert isinstance(trigger, AWSCronTrigger)

    # 3rd Monday of month (2 = MON in AWS)
    trigger = scheduler_service._parse_expression("cron(0 10 ? * 2#3 *)")
    assert isinstance(trigger, AWSCronTrigger)


def test_parse_rate_expression(scheduler_service):
    # rate(1 minute)
    trigger = scheduler_service._parse_expression("rate(1 minute)")
    assert isinstance(trigger, IntervalTrigger)
    assert trigger.interval_length == 60

    # rate(2 hours)
    trigger = scheduler_service._parse_expression("rate(2 hours)")
    assert trigger.interval_length == 7200

    # rate(1 day)
    trigger = scheduler_service._parse_expression("rate(1 day)")
    assert trigger.interval_length == 86400


def test_load_schedules(scheduler_service):
    config = {
        "func-1": {
            "events": [{"schedule": {"rate": "rate(5 minutes)", "input": '{"test": "data"}'}}]
        },
        "func-2": {"events": [{"schedule": {"rate": "cron(0 0 * * ? *)"}}]},
    }

    scheduler_service.load_schedules(config)
    jobs = scheduler_service.scheduler.get_jobs()
    assert len(jobs) == 2

    job_ids = [job.id for job in jobs]
    assert "func-1_sched_0" in job_ids
    assert "func-2_sched_0" in job_ids


@pytest.mark.asyncio
async def test_job_execution(scheduler_service, mock_invoker):
    # Manually trigger the job function to see if it calls the invoker
    scheduler_service._add_schedule_job("test_job", "my-func", "rate(1 minute)", '{"foo": "bar"}')

    job = scheduler_service.scheduler.get_job("test_job")
    # The actual function is wrapped in an async function in _add_schedule_job
    await job.func()

    mock_invoker.invoke_function.assert_called_once_with("my-func", b'{"foo": "bar"}')
