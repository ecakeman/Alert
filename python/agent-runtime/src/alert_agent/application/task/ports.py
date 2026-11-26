"""ReceiptStore 端口：幂等的任务接入（design 4.2/6.2）。

端口类型只涉及业务字段（CR-LAY-07）；行模型保留在基础设施层。``accept``
必须通过 INSERT ... ON CONFLICT (task_id, generation) DO NOTHING 实现原子性
——绝不使用先查后插（CR-REG-40，Q1）；同 generation 的 FAILED 回执会被
重新接受并保留原始失败记录（CR-TASK-03）；早于已记录 generation 的提交
以 ``STALE_GENERATION`` 拒绝且零写入（6.1.3.2）。
所有状态转换都携带 generation 条件（CR-TASK-04，CR-REG-41）。
``mark_finished`` 只接受 COMPLETED 或 FAILED（状态机 CR-TASK-04）。
"""

from __future__ import annotations

from datetime import datetime
from typing import Protocol

from pydantic import BaseModel, ConfigDict, Field

from alert_agent.domain.shared import FrozenBoundary, TaskIdentity
from alert_agent.domain.task import CallbackState, ReceiptStatus


class TaskSubmission(BaseModel):
    """应用层任务输入；与 SubmitTaskRequest 一一映射（CR-GW-03）。"""

    model_config = ConfigDict(frozen=True)

    identity: TaskIdentity
    boundary: FrozenBoundary
    service_id: str = Field(min_length=1)


class AcceptOutcome(BaseModel):
    """幂等接入的结果（CR-TASK-01）。"""

    model_config = ConfigDict(frozen=True)

    accepted: bool
    duplicate: bool


class UnfinishedReceipt(BaseModel):
    """供恢复用例消费的回执投影（CR-TASK-10）。"""

    model_config = ConfigDict(frozen=True)

    task_id: str
    generation: int
    task_type: str
    episode_id: str
    service_id: str
    thread_id: str
    status: ReceiptStatus


class ReceiptStore(Protocol):
    async def accept(
        self, submission: TaskSubmission, *, thread_id: str, accepted_at: datetime
    ) -> AcceptOutcome: ...

    async def mark_running(
        self, task_id: str, generation: int, *, started_at: datetime
    ) -> bool: ...

    async def mark_finished(
        self,
        task_id: str,
        generation: int,
        *,
        status: ReceiptStatus,
        error_code: str,
        error_message: str,
        callback_state: CallbackState,
        finished_at: datetime,
    ) -> bool: ...

    async def mark_callback_state(
        self,
        task_id: str,
        generation: int,
        callback_state: CallbackState,
        *,
        updated_at: datetime,
    ) -> bool: ...

    async def list_unfinished(self) -> list[UnfinishedReceipt]: ...

    async def list_unconfirmed_results(self) -> list[UnfinishedReceipt]: ...

class GatewayAck(BaseModel):
    """一次回调的协议层确认（design 4.3）。

    ``accepted`` 镜像 ``TaskAck.accepted``；客户端实现将其读取为每一个
    后续动作的前置条件（CR-TASK-06，Q5 教训）。
    """

    model_config = ConfigDict(frozen=True)

    accepted: bool


class ArtifactCallback(BaseModel):
    """RegisterTaskArtifact 载荷（design 4.3，CR-GW-09）。"""

    model_config = ConfigDict(frozen=True)

    identity: TaskIdentity
    artifact_type: str = Field(min_length=1)
    artifact_version: int = Field(gt=0)
    content_hash: str = Field(min_length=1)
    content_type: str = Field(min_length=1)
    storage_key: str = Field(min_length=1)
    source_refs: tuple[str, ...] = ()


class TaskFailureReport(BaseModel):
    """ReportTaskFailure 载荷；只有 RetryableError 映射为 retryable=true（CR-TASK-12/13）。"""

    model_config = ConfigDict(frozen=True)

    identity: TaskIdentity
    retryable: bool
    error_code: str = Field(min_length=1, max_length=100)
    error_message: str = Field(min_length=1)


class TaskContextView(BaseModel):
    """以业务字段表示的 GetTaskContext 响应（design 4.3）。

    ``deadline_at`` 是带时区的 UTC 时间，从 proto int64 Unix 秒解析；
    调用方将其与 SubmitTask 的边界进行比较。
    """

    model_config = ConfigDict(frozen=True)

    input_version_from: int
    input_version_to: int
    monitor_watermark: int
    app_event_watermark: int
    deadline_at: datetime
    task_type: str = Field(min_length=1)


class TaskGateway(Protocol):
    """七个 SystemTaskGatewayService 回调（design 4.3，CR-GW-04）。

    实现必须把 ``TaskAck.accepted`` 读取为下一个动作的前置条件，绝不吞掉
    异常：单次续期失败立即浮出，使 worker 可以统计失败次数并在达到阈值时
    终止（CR-REG-44）。确定性拒绝（LEASE_LOST/STALE_GENERATION 稳定码）
    抛出 FatalError；瞬态失败在有限次重试后浮出。
    """

    async def report_started(self, identity: TaskIdentity) -> GatewayAck: ...

    async def report_heartbeat(self, identity: TaskIdentity) -> GatewayAck: ...

    async def register_artifact(self, artifact: ArtifactCallback) -> GatewayAck: ...

    async def report_result(self, identity: TaskIdentity) -> GatewayAck: ...

    async def report_failure(self, failure: TaskFailureReport) -> GatewayAck: ...

    async def get_task_context(self, identity: TaskIdentity) -> TaskContextView: ...

    async def abandon_task(self, identity: TaskIdentity) -> GatewayAck: ...

class ArtifactRegistration(BaseModel):
    """cognitive.episode_artifacts 的注册载荷（design 4.6）。

    ``artifact_version`` 是期望版本：UNIQUE
    (episode_id, artifact_type, artifact_version) 约束是乐观锁的基础
    （CR-DIAG-19，CR-REG-46）。
    """

    model_config = ConfigDict(frozen=True)

    task_id: str = Field(min_length=1)
    generation: int = Field(gt=0)
    episode_id: str = Field(min_length=1)
    artifact_type: str = Field(min_length=1)
    artifact_version: int = Field(gt=0)
    content_hash: str = Field(min_length=1)
    content_type: str = Field(min_length=1)
    storage_key: str = Field(min_length=1)


class ArtifactRecord(BaseModel):
    """以业务字段表示的已注册产物视图（无内容列，D5）。"""

    model_config = ConfigDict(frozen=True)

    artifact_id: str = Field(min_length=1)
    task_id: str = Field(min_length=1)
    generation: int = Field(gt=0)
    episode_id: str = Field(min_length=1)
    artifact_type: str = Field(min_length=1)
    artifact_version: int = Field(gt=0)
    content_hash: str = Field(min_length=1)
    content_type: str = Field(min_length=1)
    storage_key: str = Field(min_length=1)


class ArtifactStore(Protocol):
    """cognitive.episode_artifacts 上的版本注册表（design 4.6，CR-REG-46）。

    ``register`` 对同 generation + 同 hash 幂等（Q7），以
    ``ARTIFACT_VERSION_CONFLICT`` 拒绝更旧的 generation（更旧的
    generation 永远不会被覆盖），并把任何其他版本冲突视为冲突；
    ``latest_version`` 驱动单调的版本分配（CR-DIAG-19）。
    """

    async def register(self, registration: ArtifactRegistration) -> ArtifactRecord: ...

    async def latest_version(self, episode_id: str, artifact_type: str) -> int:
        """单个 episode 产物类型的最高已注册版本；无记录时为 0。"""
        ...

    async def find(
        self, episode_id: str, artifact_type: str, artifact_version: int
    ) -> ArtifactRecord | None: ...