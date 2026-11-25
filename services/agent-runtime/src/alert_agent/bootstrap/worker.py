"""Worker runtime: bounded-queue consumers, heartbeat renewal, recovery scan
(design 4.1/4.2, CR-TASK-06~11).

Role split stays narrow (CR-REG-27, avoiding the god-object): ``TaskRunner``
owns one task's lifecycle -- receipt RUNNING, heartbeat monitor with
failure-threshold cancellation, deadline enforcement, terminal receipt write;
``TaskWorkerRuntime`` owns the loops; ``EnqueueingSubmit`` bridges the submit
use case and the bounded queue. In-flight cleanup happens in the innermost
finally of the per-task run (CR-TASK-09, CR-REG-45). Heartbeat failures are
logged with ``LOGGER.exception`` and counted, never swallowed (CR-REG-47);
reaching the threshold cancels the executor task and the receipt records the
termination (CR-TASK-06/07).

The executor is late-bound from bootstrap wiring: diagnosis/archive executors
land with T13/T14. Until then ``start()`` keeps every loop idle with an
explicit warning -- no fake execution (CR-DISC-05).
"""

from __future__ import annotations

import asyncio
import logging
from dataclasses import dataclass

import grpc.aio
from psycopg_pool import AsyncConnectionPool

from alert_agent.application.shared.ports import Clock
from alert_agent.application.task.executor import TaskExecutor
from alert_agent.application.task.ports import (
    AcceptOutcome,
    ReceiptStore,
    TaskGateway,
    TaskSubmission,
    UnfinishedReceipt,
)
from alert_agent.application.task.queue import BoundedTaskQueue, QueuedTask, TaskQueue
from alert_agent.application.task.recover import RecoverUnfinished
from alert_agent.application.task.submit import SubmitTask
from alert_agent.bootstrap.wiring import (
    SystemClock,
    build_gateway_client,
    build_receipt_store,
)
from alert_agent.config import Settings
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError, TaskIdentity
from alert_agent.domain.task import CallbackState, ReceiptStatus

LOGGER = logging.getLogger(__name__)

_TASK_KEY = tuple[str, int]


@dataclass
class TaskRunner:
    """One task's lifecycle: RUNNING -> executor -> terminal receipt."""

    store: ReceiptStore
    gateway: TaskGateway
    executor: TaskExecutor | None
    clock: Clock
    heartbeat_interval_seconds: float
    heartbeat_failure_threshold: int

    def __post_init__(self) -> None:
        self._in_flight: set[_TASK_KEY] = set()

    async def run(self, run: QueuedTask) -> None:
        """Execute one queued task with full exception isolation (CR-TASK-09)."""
        if run.submission is not None:
            key: _TASK_KEY = (run.submission.identity.task_id, run.submission.identity.generation)
        elif run.receipt is not None:
            key = (run.receipt.task_id, run.receipt.generation)
        else:  # unreachable: QueuedTask validates exactly one source
            return
        self._in_flight.add(key)
        stop = asyncio.Event()
        heartbeat_task: asyncio.Task[None] | None = None
        exec_task: asyncio.Task[None] | None = None
        try:
            if not await self._store.mark_running(key[0], key[1], started_at=self._clock.now()):
                # Older generation or already advanced (CR-REG-41 generation condition).
                LOGGER.info("receipt %s gen %d no longer ACCEPTED; skip run", *key)
                return
            if run.submission is None:
                await self._finish_unrenewable(run.receipt)  # type: ignore[arg-type]
                return
            submission = run.submission
            exec_task = asyncio.create_task(self._executor.execute(submission))
            heartbeat_task = asyncio.create_task(
                self._heartbeat_loop(submission.identity, exec_task, stop)
            )
            remaining = (submission.boundary.deadline_at - self._clock.now()).total_seconds()
            done, _pending = await asyncio.wait({exec_task}, timeout=max(remaining, 0.0))
            if exec_task not in done:
                # Total-duration deadline (5.1.5, CR-REG-38): stop and record.
                exec_task.cancel()
                await asyncio.wait({exec_task})
                await self._finish(
                    key,
                    ReceiptStatus.FAILED,
                    "DEADLINE_EXCEEDED",
                    "task exceeded its deadline_at",
                    CallbackState.NOT_SENT,
                )
            elif exec_task.cancelled():
                # Heartbeat threshold cancellation (CR-TASK-06/07, CR-REG-44).
                await self._finish(
                    key,
                    ReceiptStatus.FAILED,
                    ErrorCode.LEASE_LOST.value,
                    "heartbeat renewal threshold reached; execution cancelled",
                    CallbackState.NOT_SENT,
                )
            else:
                exc = exec_task.exception()
                if exc is None:
                    await self._finish(
                        key, ReceiptStatus.COMPLETED, "", "", CallbackState.SENT
                    )
                else:
                    error_code, error_message = _classify(exc)
                    await self._finish(
                        key, ReceiptStatus.FAILED, error_code, error_message, CallbackState.NOT_SENT
                    )

        except Exception:
            LOGGER.exception("task runner crashed for task %s gen %d", *key)
        finally:
            stop.set()
            for task in (heartbeat_task, exec_task):
                if task is not None and not task.done():
                    task.cancel()
            for task in (heartbeat_task, exec_task):
                if task is not None:
                    await asyncio.wait({task})
            self._in_flight.discard(key)

    def in_flight(self) -> set[_TASK_KEY]:
        return set(self._in_flight)

    async def _heartbeat_loop(
        self, identity: TaskIdentity, exec_task: asyncio.Task[None], stop: asyncio.Event
    ) -> None:
        """Renew at <= lease/3; count failures, cancel at threshold (CR-TASK-06/07)."""
        failures = 0
        while not stop.is_set():
            try:
                await asyncio.wait_for(stop.wait(), timeout=self.heartbeat_interval_seconds)
            except TimeoutError:
                pass
            if stop.is_set():
                return
            try:
                ack = await self.gateway.report_heartbeat(identity)
                failures = 0 if ack.accepted else failures + 1
                if not ack.accepted:
                    LOGGER.warning(
                        "heartbeat rejected for task %s gen %d (%d/%d)",
                        identity.task_id,
                        identity.generation,
                        failures,
                        self.heartbeat_failure_threshold,
                    )
            except Exception:
                # CancelledError propagates (not an Exception); every renewal
                # failure is logged, never swallowed (CR-REG-47).
                failures += 1
                LOGGER.exception(
                    "heartbeat renewal failed for task %s gen %d (%d/%d)",
                    identity.task_id,
                    identity.generation,
                    failures,
                    self.heartbeat_failure_threshold,
                )
            if failures >= self.heartbeat_failure_threshold:
                LOGGER.error(
                    "heartbeat threshold reached for task %s gen %d; cancelling execution",
                    identity.task_id,
                    identity.generation,
                )
                exec_task.cancel()
                return

    async def _finish_unrenewable(self, receipt: UnfinishedReceipt) -> None:
        """Recovered run without a persisted lease/boundary (migration 0001):
        advance to a retryable terminal state so Go re-dispatches a fresh
        generation with a new lease (5.2.2 "executable or retryable state")."""
        LOGGER.warning(
            "recovered task %s gen %d cannot renew: lease token is not persisted; needs re-dispatch",
            receipt.task_id,
            receipt.generation,
        )
        await self._finish(
            (receipt.task_id, receipt.generation),
            ReceiptStatus.FAILED,
            ErrorCode.LEASE_LOST.value,
            "lease token not persisted after restart; task needs re-dispatch",
            CallbackState.NOT_SENT,
        )

    async def _finish(
        self,
        key: _TASK_KEY,
        status: ReceiptStatus,
        error_code: str,
        error_message: str,
        callback_state: CallbackState,
    ) -> None:
        updated = await self.store.mark_finished(
            key[0],
            key[1],
            status=status,
            error_code=error_code,
            error_message=error_message,
            callback_state=callback_state,
            finished_at=self.clock.now(),
        )
        if not updated:
            LOGGER.warning(
                "receipt %s gen %d could not advance to %s (generation condition)", *key, status.value
            )


def _classify(exc: BaseException) -> tuple[str, str]:
    """Short error code + readable message; only domain failures carry codes (CR-TASK-12/13)."""
    if isinstance(exc, (RetryableError, FatalError)):
        return exc.code.value, exc.message
    return "EXECUTOR_ERROR", str(exc)[:500]


class EnqueueingSubmit:
    """Submit use case + bounded-queue enqueue (design 4.2, CR-TASK-08).

    Receipt persistence is the acceptance fact. Enqueue failure keeps the
    "awaiting recovery" fact and reports ``accepted=false`` (6.1.3.1) instead
    of a fake success; duplicates never re-enqueue (CR-TASK-01).
    """

    def __init__(self, submit: SubmitTask, queue: TaskQueue) -> None:
        self._submit = submit
        self._queue = queue

    async def execute(self, submission: TaskSubmission) -> AcceptOutcome:
        outcome = await self._submit.execute(submission)
        if (
            outcome.accepted
            and not outcome.duplicate
            and not self._queue.try_enqueue(QueuedTask(submission=submission))
        ):
            LOGGER.warning(
                "queue full; task %s gen %d stays awaiting recovery (accepted=false)",
                submission.identity.task_id,
                submission.identity.generation,
            )
            return AcceptOutcome(accepted=False, duplicate=False)
        return outcome


class TaskWorkerRuntime:
    """Owns the queue-consumer and recovery-scan loops (CR-REG-27 role split)."""

    def __init__(
        self,
        queue: TaskQueue,
        runner: TaskRunner,
        recover: RecoverUnfinished,
        *,
        worker_concurrency: int,
        recovery_scan_interval_seconds: float,
    ) -> None:
        self._queue = queue
        self._runner = runner
        self._recover = recover
        self._worker_concurrency = worker_concurrency
        self._recovery_interval = recovery_scan_interval_seconds

    async def start(self) -> list[asyncio.Task[None]]:
        """Create the loop tasks; idle when the executor is not wired yet (T13/T14)."""
        if self._runner.executor is None:
            LOGGER.warning(
                "task executor is not wired yet (T13/T14 wiring); worker loops stay idle"
            )
            return []
        tasks = [
            asyncio.create_task(self._consumer_loop(index), name=f"task-consumer-{index}")
            for index in range(self._worker_concurrency)
        ]
        tasks.append(asyncio.create_task(self._recovery_loop(), name="task-recovery-scan"))
        return tasks

    async def _consumer_loop(self, index: int) -> None:
        while True:
            run = await self._queue.dequeue()
            try:
                await self._runner.run(run)
            except Exception:
                # Isolation belt: TaskRunner.run already guards; this keeps the
                # consumer alive no matter what (CR-TASK-09).
                LOGGER.exception("consumer %d: task run escaped the runner guard", index)

    async def _recovery_loop(self) -> None:
        while True:
            try:
                recovered = await self._recover.execute()
                if recovered:
                    LOGGER.info("recovery scan re-enqueued %d unfinished tasks", recovered)
            except asyncio.CancelledError:
                raise
            except Exception:
                LOGGER.exception("recovery scan failed")  # CR-REG-47: never silent
            await asyncio.sleep(self._recovery_interval)


def build_task_worker_runtime(
    settings: Settings,
    pool: AsyncConnectionPool,
    channel: grpc.aio.Channel,
    queue: BoundedTaskQueue,
) -> TaskWorkerRuntime:
    """Assemble queue/runner/recovery with the wired gateway client and store.

    ``queue`` is passed in by the lifespan so the submit path and the
    consumers share one bounded instance (CR-TASK-08). The executor is
    late-bound from wiring (``resolve_task_executor``) and is None until
    T13/T14 land; loops stay idle meanwhile (CR-DISC-05).
    """
    executor = _resolve_task_executor()
    store = build_receipt_store(pool)
    gateway = build_gateway_client(channel, settings)
    runner = TaskRunner(
        store=store,
        gateway=gateway,
        executor=executor,
        clock=SystemClock(),
        heartbeat_interval_seconds=settings.gateway.heartbeat_interval_seconds,
        heartbeat_failure_threshold=settings.gateway.heartbeat_failure_threshold,
    )
    recover = RecoverUnfinished(store, queue)
    return TaskWorkerRuntime(
        queue=queue,
        runner=runner,
        recover=recover,
        worker_concurrency=settings.gateway.worker_concurrency,
        recovery_scan_interval_seconds=settings.gateway.recovery_scan_interval_seconds,
    )


def _resolve_task_executor() -> TaskExecutor | None:
    """Late-bind the executor from wiring (T13/T14); None keeps loops idle.

    Late import keeps T08 static-verifiable before the executor modules exist.
    """
    try:
        from alert_agent.bootstrap.wiring import resolve_task_executor
    except ImportError:
        return None
    return resolve_task_executor()