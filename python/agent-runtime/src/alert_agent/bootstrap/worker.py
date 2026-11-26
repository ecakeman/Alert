"""Worker 运行时：有界队列消费方、心跳续租、恢复扫描
（design 4.1/4.2，CR-TASK-06~11）。

角色拆分保持狭窄（CR-REG-27，规避上帝对象）：``TaskRunner`` 掌管单个任务
的生命周期——回执置 RUNNING、带失败阈值取消的心跳监控、deadline 强制执行、
终态回执写入；``TaskWorkerRuntime`` 掌管各循环；``EnqueueingSubmit`` 在提交
用例与有界队列之间架桥。执行中的清理发生在单任务运行最内层的 finally 中
（CR-TASK-09，CR-REG-45）。心跳失败以 ``LOGGER.exception`` 记录并计数，
绝不吞掉（CR-REG-47）；达到阈值会取消执行器任务，回执记录该终止
（CR-TASK-06/07）。

执行器由 bootstrap 装配延迟绑定：diagnosis/archive 执行器随 T13/T14 落地。
在此之前 ``start()`` 使所有循环保持空闲并输出显式警告——不做虚假执行
（CR-DISC-05）。
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
    build_observation_sink,
    build_receipt_store,
)
from alert_agent.config import Settings
from alert_agent.domain.shared import ErrorCode, FatalError, RetryableError, TaskIdentity
from alert_agent.domain.task import CallbackState, ReceiptStatus

LOGGER = logging.getLogger(__name__)

_TASK_KEY = tuple[str, int]


@dataclass
class TaskRunner:
    """单个任务的生命周期：RUNNING -> 执行器 -> 终态回执。"""

    store: ReceiptStore
    gateway: TaskGateway
    executor: TaskExecutor | None
    clock: Clock
    heartbeat_interval_seconds: float
    heartbeat_failure_threshold: int

    def __post_init__(self) -> None:
        self._in_flight: set[_TASK_KEY] = set()

    async def run(self, run: QueuedTask) -> None:
        """以完整的异常隔离执行一个排队任务（CR-TASK-09）。"""
        submission = run.submission
        if submission is not None:
            key: _TASK_KEY = (submission.identity.task_id, submission.identity.generation)
        elif run.receipt is not None:
            key = (run.receipt.task_id, run.receipt.generation)
            submission = run.receipt.to_submission()
            if submission is None:
                self._in_flight.add(key)
                try:
                    await self._finish_unrenewable(run.receipt)
                finally:
                    self._in_flight.discard(key)
                return
            if run.receipt.status == ReceiptStatus.COMPLETED:
                self._in_flight.add(key)
                try:
                    await self._replay_result(run.receipt, submission)
                finally:
                    self._in_flight.discard(key)
                return
        else:  # 不可达：QueuedTask 校验恰好有一个来源
            return
        self._in_flight.add(key)
        stop = asyncio.Event()
        heartbeat_task: asyncio.Task[None] | None = None
        exec_task: asyncio.Task[None] | None = None
        try:
            if not await self.store.mark_running(key[0], key[1], started_at=self.clock.now()):
                # 更旧 generation 或已推进（CR-REG-41 generation 条件）。
                LOGGER.info("receipt %s gen %d no longer ACCEPTED; skip run", *key)
                return
            exec_task = asyncio.create_task(self.executor.execute(submission))
            heartbeat_task = asyncio.create_task(
                self._heartbeat_loop(submission.identity, exec_task, stop)
            )
            remaining = (submission.boundary.deadline_at - self.clock.now()).total_seconds()
            done, _pending = await asyncio.wait({exec_task}, timeout=max(remaining, 0.0))
            if exec_task not in done:
                # 总时长 deadline（5.1.5，CR-REG-38）：停止并记录。
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
                # 心跳阈值取消（CR-TASK-06/07，CR-REG-44）。
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
        """以 <= lease/3 的间隔续租；失败计数，达到阈值即取消
        （CR-TASK-06/07）。"""
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
                # CancelledError 会向上传播（不继承 Exception）；每次续租
                # 失败都被记录，绝不吞掉（CR-REG-47）。
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
        """回执无法重建可调用身份时的显式降级（P5.9 GAP-1）。

        仅两种合法原因：
        (a) 六列缺失（迁移前老回执）→ LEASE_CREDENTIALS_ABSENT；
        (b) deadline_at 已过 → DEADLINE_EXCEEDED。
        两者都以 WARN 带原因码记录，绝不静默。
        """
        if receipt.deadline_at is not None and receipt.deadline_at <= self.clock.now():
            LOGGER.warning(
                "recovered task %s gen %d cannot resume: DEADLINE_EXCEEDED at restart",
                receipt.task_id,
                receipt.generation,
            )
            await self._finish(
                (receipt.task_id, receipt.generation),
                ReceiptStatus.FAILED,
                "DEADLINE_EXCEEDED",
                "task exceeded its deadline_at at restart",
                CallbackState.NOT_SENT,
            )
            return
        LOGGER.warning(
            "recovered task %s gen %d cannot renew: lease credentials absent"
            " (LEASE_CREDENTIALS_ABSENT); needs re-dispatch",
            receipt.task_id,
            receipt.generation,
        )
        await self._finish(
            (receipt.task_id, receipt.generation),
            ReceiptStatus.FAILED,
            ErrorCode.LEASE_LOST.value,
            "lease credentials absent after restart; task needs re-dispatch",
            CallbackState.NOT_SENT,
        )

    async def _replay_result(
        self, receipt: UnfinishedReceipt, submission: TaskSubmission
    ) -> None:
        """COMPLETED + NOT_SENT 的结果回调重放（P5.9 GAP-1）。

        仅在 token/边界齐备（to_submission 成功）时进入；拒绝或异常都不改变
        NOT_SENT，下一轮扫描继续重试（CR-REG-47）。
        """
        try:
            ack = await self.gateway.report_result(submission.identity)
        except Exception:
            LOGGER.exception(
                "result replay failed for task %s gen %d; stays NOT_SENT",
                receipt.task_id,
                receipt.generation,
            )
            return
        if ack.accepted:
            await self.store.mark_callback_state(
                receipt.task_id,
                receipt.generation,
                CallbackState.SENT,
                updated_at=self.clock.now(),
            )
            LOGGER.info(
                "replayed result callback for task %s gen %d",
                receipt.task_id,
                receipt.generation,
            )
        else:
            LOGGER.warning(
                "result replay rejected for task %s gen %d; stays NOT_SENT",
                receipt.task_id,
                receipt.generation,
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
    """短错误码 + 可读消息；仅领域失败携带错误码（CR-TASK-12/13）。"""
    if isinstance(exc, (RetryableError, FatalError)):
        return exc.code.value, exc.message
    return "EXECUTOR_ERROR", str(exc)[:500]


class EnqueueingSubmit:
    """提交用例 + 有界队列入队（design 4.2，CR-TASK-08）。

    回执持久化即受理事实。入队失败保持"待恢复"事实并报告
    ``accepted=false``（6.1.3.1），而非虚假成功；重复任务绝不重新入队
    （CR-TASK-01）。
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
    """掌管队列消费与恢复扫描循环（CR-REG-27 角色拆分）。"""

    def __init__(
        self,
        queue: TaskQueue,
        runner: TaskRunner,
        recover: RecoverUnfinished,
        *,
        worker_concurrency: int,
        recovery_scan_interval_seconds: float,
        observation_sink=None,
    ) -> None:
        self._queue = queue
        self._runner = runner
        self._recover = recover
        self._worker_concurrency = worker_concurrency
        self._recovery_interval = recovery_scan_interval_seconds
        self._observation_sink = observation_sink

    async def start(self) -> list[asyncio.Task[None]]:
        """创建循环任务；执行器尚未接线时保持空闲（T13/T14）。"""
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
        if self._observation_sink is not None:
            # T23：面向 cognitive.llm_calls 的后台批量刷写（CR-REG-19）。
            tasks.append(
                asyncio.create_task(
                    self._observation_sink.flush_forever(), name="observe-flush"
                )
            )
        return tasks

    async def _consumer_loop(self, index: int) -> None:
        while True:
            run = await self._queue.dequeue()
            try:
                await self._runner.run(run)
            except Exception:
                # 隔离保险：TaskRunner.run 已有防护；这里保证无论如何
                # 消费方都存活（CR-TASK-09）。
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
                LOGGER.exception("recovery scan failed")  # CR-REG-47：绝不静默
            await asyncio.sleep(self._recovery_interval)


def build_task_worker_runtime(
    settings: Settings,
    pool: AsyncConnectionPool,
    channel: grpc.aio.Channel,
    queue: BoundedTaskQueue,
) -> TaskWorkerRuntime:
    """使用已接线的 gateway 客户端与存储装配队列/runner/恢复。

    ``queue`` 由 lifespan 传入，使提交路径与消费方共用同一个有界实例
    （CR-TASK-08）。执行器由装配侧延迟绑定（``resolve_task_executor``），
    在 T13/T14 落地之前为 None；期间各循环保持空闲（CR-DISC-05）。
    """
    executor = _resolve_task_executor()
    store = build_receipt_store(pool)
    gateway = build_gateway_client(channel, settings)
    observation_sink = build_observation_sink(pool)
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
        observation_sink=observation_sink,
    )


def _resolve_task_executor() -> TaskExecutor | None:
    """从装配侧延迟绑定执行器（T13/T14）；为 None 时循环保持空闲。

    延迟导入使 T08 在执行器模块存在之前即可静态验证。
    """
    try:
        from alert_agent.bootstrap.wiring import resolve_task_executor
    except ImportError:
        return None
    return resolve_task_executor()