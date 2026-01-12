import json
import logging
import re
from typing import Any, Dict, Optional

from apscheduler.schedulers.asyncio import AsyncIOScheduler
from apscheduler.triggers.base import BaseTrigger
from apscheduler.triggers.interval import IntervalTrigger
from aws_croniter import AwsCroniter

from services.gateway.services.lambda_invoker import LambdaInvoker

logger = logging.getLogger("gateway.scheduler")


class AWSCronTrigger(BaseTrigger):
    """APScheduler trigger that uses aws-croniter to support full AWS Cron syntax."""

    def __init__(self, expression: str):
        # Extract the content inside cron(...)
        match = re.match(r"cron\((.+)\)", expression)
        if not match:
            raise ValueError(f"Invalid cron expression: {expression}")
        self.expr = match.group(1)
        # Validate expression early
        try:
            AwsCroniter(self.expr)
        except Exception as e:
            raise ValueError(f"Invalid AWS cron syntax '{self.expr}': {e}") from e

    def get_next_fire_time(self, previous_fire_time, now):
        # aws-croniter expects a datetime and returns the next one
        # Note: AwsCroniter.get_next() returns a list of n occurrences (defaults to [1])
        cron = AwsCroniter(self.expr)
        next_times = cron.get_next(now)
        return next_times[0] if next_times else None

    def __str__(self):
        return f"aws_cron({self.expr})"


class SchedulerService:
    def __init__(self, invoker: LambdaInvoker):
        self.invoker = invoker
        self.scheduler = AsyncIOScheduler()
        self._jobs = {}

    async def start(self):
        """Start the scheduler."""
        if not self.scheduler.running:
            self.scheduler.start()
            logger.info("Scheduler service started.")

    async def stop(self):
        """Stop the scheduler."""
        if self.scheduler.running:
            self.scheduler.shutdown()
            logger.info("Scheduler service stopped.")

    def load_schedules(self, functions_config: Dict[str, Any]):
        """Load schedules from functions configuration."""
        # Remove existing jobs managed by this loader
        self.scheduler.remove_all_jobs()

        for func_name, config in functions_config.items():
            events = config.get("events", [])
            for i, event in enumerate(events):
                if "schedule" in event:
                    sched_config = event["schedule"]
                    expression = sched_config.get("rate") or sched_config.get("expression")
                    input_data = sched_config.get("input")

                    if not expression:
                        continue

                    job_id = f"{func_name}_sched_{i}"
                    try:
                        self._add_schedule_job(job_id, func_name, expression, input_data)
                    except Exception as e:
                        logger.error(f"Failed to add schedule for {func_name} ({expression}): {e}")

    def _add_schedule_job(
        self, job_id: str, function_name: str, expression: str, input_data: Optional[str]
    ):
        """Parse expression and add job to scheduler."""
        trigger = self._parse_expression(expression)

        # Prepare payload
        payload = b"{}"
        if input_data:
            if isinstance(input_data, str):
                payload = input_data.encode("utf-8")
            else:
                payload = json.dumps(input_data).encode("utf-8")

        async def job_func():
            logger.info(f"Triggering scheduled invocation for {function_name}")
            try:
                # We use a large timeout for scheduled tasks or config value
                await self.invoker.invoke_function(function_name, payload)
            except Exception as e:
                logger.error(f"Scheduled invocation failed for {function_name}: {e}")

        self.scheduler.add_job(
            job_func, trigger=trigger, id=job_id, replace_existing=True, misfire_grace_time=60
        )
        logger.info(f"Added schedule job {job_id} for {function_name}: {expression}")

    def _parse_expression(self, expression: str) -> Any:
        """Parse AWS schedule expression (cron or rate)."""
        # cron(Minutes Hours Day-of-month Month Day-of-week Year)
        if expression.startswith("cron("):
            return AWSCronTrigger(expression)

        # rate(value unit)
        rate_match = re.match(r"rate\((\d+)\s+(minute|minutes|hour|hours|day|days)\)", expression)
        if rate_match:
            value = int(rate_match.group(1))
            unit = rate_match.group(2)

            if "minute" in unit:
                return IntervalTrigger(minutes=value)
            elif "hour" in unit:
                return IntervalTrigger(hours=value)
            elif "day" in unit:
                return IntervalTrigger(days=value)

        raise ValueError(f"Unsupported schedule expression format: {expression}")
