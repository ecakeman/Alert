"""集中式运行时配置，带快速失败校验。

唯一的 pydantic-settings 入口（CR-BOOT-01）：分组的冻结模型组装为
``Settings``；每个键、默认值、取值范围与超时仅声明
一次（CR-BOOT-02，CR-REG-29，CR-REG-32——无重复命名，无 ``or
default`` 吞掉 ``0`` 的情况）。``load_settings`` 是唯一受支持的入口
点：对必填键的占位值予以拒绝，并给出脱敏后的
错误——只列出变量名而绝不显示值（CR-BOOT-03，6.9.2.1），
同时强制跨组一致性（grpc/http 监听端口冲突 6.9.2.2、
heartbeat 与 lease/ownership 的关系、恢复扫描周期）。所有实例均为
冻结：启动之后运行时代码绝不变更配置
（CR-BOOT-02）。
"""

from __future__ import annotations

from pathlib import Path
from urllib.parse import urlsplit

from pydantic import BaseModel, ConfigDict, Field, ValidationError, field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

_ENV_FILE = ".env"
_ENV_ENCODING = "utf-8"

DB_SCHEMES = frozenset({"postgres", "postgresql"})

PLACEHOLDER_VALUES = frozenset(
    {"", "changeme", "change-me", "change_me", "placeholder", "unset", "todo", "fixme", "your-token", "your-key", "xxx"}
)

_PLACEHOLDER_CHECKS: tuple[tuple[str, tuple[str, ...]], ...] = (
    ("shared", ("database_url_cognitive",)),
    ("gateway", ("agent_grpc_target", "agent_grpc_listen", "http_listen", "internal_token")),
    ("diagnosis", ("mcp_endpoint", "mcp_token", "knowledge_a2a_endpoint")),
    ("ingestion", ("ingest_file_root",)),
    ("llm", ("llm_api_base", "llm_api_key", "llm_model")),
)


class ConfigError(Exception):
    """启动配置失败；消息只点名变量，绝不显示值。"""


class _FrozenModel(BaseModel):
    model_config = ConfigDict(frozen=True)


class _GroupSettings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=_ENV_FILE,
        env_file_encoding=_ENV_ENCODING,
        frozen=True,
        extra="ignore",
    )


class SharedSettings(_GroupSettings):
    log_level: str = Field(default="INFO")
    database_url_cognitive: str = Field(description="Postgres DSN for the cognitive schema (CR-DATA-01)")
    pool_min_size: int = Field(default=1, ge=1)
    pool_max_size: int = Field(default=10, ge=1, le=100)
    shutdown_timeout_seconds: int = Field(default=20, ge=1, le=20)
    observe_queue_size: int = Field(default=1024, ge=1)
    observe_batch_size: int = Field(default=100, ge=1)
    observe_flush_interval_seconds: float = Field(default=5.0, gt=0)

    @field_validator("log_level")
    @classmethod
    def _check_log_level(cls, value: str) -> str:
        allowed = {"DEBUG", "INFO", "WARNING", "ERROR"}
        if value.strip().upper() not in allowed:
            raise ValueError("LOG_LEVEL must be one of DEBUG/INFO/WARNING/ERROR")
        return value.strip().upper()

    @model_validator(mode="after")
    def _check_pool_bounds(self) -> SharedSettings:
        if self.pool_min_size > self.pool_max_size:
            raise ValueError("POOL_MIN_SIZE must be <= POOL_MAX_SIZE")
        return self


class GatewaySettings(_GroupSettings):
    agent_grpc_target: str = Field(description="Go control-plane gateway endpoint for the 7 callbacks")
    agent_grpc_listen: str = Field(description="gRPC server listen address, host:port (must differ from HTTP)")
    http_listen: str = Field(description="HTTP server listen address, host:port (must differ from gRPC)")
    internal_token: str = Field(description="Internal token for HTTP auth (constant-time compared)")
    gateway_rpc_timeout_ms: int = Field(default=3000, ge=1)
    gateway_rpc_max_retries: int = Field(default=3, ge=0)
    heartbeat_interval_seconds: float = Field(default=10.0, gt=0)
    heartbeat_failure_threshold: int = Field(default=3, ge=1)
    task_lease_seconds: float = Field(default=30.0, gt=0)
    ownership_ttl_seconds: float = Field(default=60.0, gt=0)
    recovery_scan_interval_seconds: float = Field(default=30.0, gt=0, le=30)
    worker_concurrency: int = Field(default=4, ge=1)
    queue_capacity: int = Field(default=128, ge=1)

    @field_validator("agent_grpc_listen", "http_listen")
    @classmethod
    def _check_listen(cls, value: str, info: object) -> str:
        field_name = getattr(info, "field_name", "listen address")
        host, sep, port = value.strip().rpartition(":")
        if not sep or not host or not port.isdigit() or not 1 <= int(port) <= 65535:
            raise ValueError(f"{field_name} must be host:port with port in 1..65535")
        return value.strip()

    @model_validator(mode="after")
    def _check_heartbeat_bounds(self) -> GatewaySettings:
        if self.heartbeat_interval_seconds > self.task_lease_seconds / 3:
            raise ValueError("HEARTBEAT_INTERVAL_SECONDS must be <= TASK_LEASE_SECONDS / 3 (5.1.6)")
        if self.heartbeat_interval_seconds >= self.ownership_ttl_seconds:
            raise ValueError("HEARTBEAT_INTERVAL_SECONDS must be < OWNERSHIP_TTL_SECONDS (5.1.6)")
        return self


class DiagnosisSettings(_GroupSettings):
    diagnosis_max_tool_rounds: int = Field(default=6, ge=1, description="D10 diagnosis tool-round budget")
    diagnosis_max_llm_calls: int = Field(default=8, ge=1)
    diagnosis_deadline_ratio: float = Field(default=0.8, gt=0, le=1)
    diagnosis_max_no_progress: int = Field(default=2, ge=1)
    fault_search_top_k: int = Field(default=5, ge=1, description="CR-ARCH-07 default fault-doc topK")
    prompt_timeline_budget_chars: int = Field(default=4000, ge=1)
    prompt_search_budget_chars: int = Field(default=4000, ge=1)
    prompt_knowledge_budget_chars: int = Field(default=4000, ge=1)
    prompt_mcp_budget_chars: int = Field(default=4000, ge=1)
    prompt_summary_items: int = Field(default=10, ge=1)
    prompt_actions_items: int = Field(default=10, ge=1)
    prompt_last_error_max_chars: int = Field(default=500, ge=1)
    mcp_endpoint: str = Field(description="MCP Streamable HTTP endpoint")
    mcp_token: str = Field(description="MCP bearer token")
    mcp_call_timeout_seconds: float = Field(default=30.0, gt=0)
    mcp_max_retries: int = Field(default=2, ge=0)
    mcp_tool_catalog_ttl_seconds: float = Field(default=300.0, gt=0)
    knowledge_a2a_endpoint: str = Field(description="knowledge agent A2A endpoint (CR-GW-08a)")
    knowledge_timeout_ms: int = Field(default=20000, ge=1, description="CR-KNOW-05 single timeout source")
    knowledge_max_retries: int = Field(default=2, ge=0)
    knowledge_top_k: int = Field(default=8, ge=1, le=20, description="CR-KNOW-03 explicit topK")
    knowledge_require_citations: bool = Field(default=True, description="CR-KNOW-03 explicit constraint")


class RetrievalSettings(_GroupSettings):
    retrieval_max_hops: int = Field(default=3, ge=1, description="D10 RAG hop budget")
    retrieval_max_llm_calls: int = Field(default=12, ge=1)
    retrieval_deadline_seconds: float = Field(default=90.0, gt=0)
    retrieval_max_zero_new_hits: int = Field(default=1, ge=0)
    rrf_k: int = Field(default=60, ge=1)
    vector_top_k: int = Field(default=8, ge=1)
    fusion_top_k: int = Field(default=10, ge=1)
    rerank_top_k: int = Field(default=5, ge=1)
    grade_threshold: int = Field(default=6, ge=0, le=10)
    search_top_k_max: int = Field(default=20, ge=1, description="/search topK upper bound (7.4.4)")
    parent_expand_max_chunks: int = Field(default=8, ge=1, description="CR-RAG-18 parent expansion bound")
    parent_expand_max_tokens: int = Field(default=900, ge=1)
    refusal_message: str = Field(
        default="依据现有资料无法回答，请换问法或补充资料。",
        description="Fixed refusal wording (design 3.2.4, configurable)",
    )

    @model_validator(mode="after")
    def _check_rerank_subset(self) -> RetrievalSettings:
        if self.rerank_top_k > self.fusion_top_k:
            raise ValueError("RERANK_TOP_K must be <= FUSION_TOP_K")
        return self


class IngestionSettings(_GroupSettings):
    ingest_file_root: str = Field(description="Whitelist root directory for ingest file paths (5.3.4)")
    callback_host_allowlist: str = Field(default="localhost,127.0.0.1")
    chunk_size_tokens: int = Field(default=500, ge=1)
    chunk_overlap_tokens: int = Field(default=50, ge=0)
    ingest_callback_timeout_seconds: float = Field(default=10.0, gt=0)
    ingest_callback_max_retries: int = Field(default=3, ge=0)
    ingest_poll_interval_seconds: float = Field(default=5.0, gt=0)

    @model_validator(mode="after")
    def _check_chunk_bounds(self) -> IngestionSettings:
        if self.chunk_overlap_tokens >= self.chunk_size_tokens:
            raise ValueError("CHUNK_OVERLAP_TOKENS must be < CHUNK_SIZE_TOKENS")
        return self


class LlmSettings(_GroupSettings):
    llm_api_base: str = Field(description="OpenAI-compatible base URL (D7)")
    llm_api_key: str = Field(description="LLM API key; missing key fails fast, no scripted fallback (R3)")
    llm_model: str = Field(description="Model name for diagnosis/synthesis decisions")
    fast_temperature: float = Field(default=0.1, ge=0, le=2, description="CR-DIAG-08 explicit low temperature")
    llm_connect_timeout_seconds: float = Field(default=10.0, gt=0)
    llm_read_timeout_seconds: float = Field(default=60.0, gt=0)
    llm_max_retries: int = Field(default=2, ge=0)
    llm_retry_backoff_seconds: float = Field(default=1.0, ge=0)
    llm_json_reask_attempts: int = Field(default=1, ge=0, le=1, description="CR-REG-13 re-ask once")
    enable_embedding: bool = Field(default=True)
    embedding_api_base: str = Field(default="")
    embedding_api_key: str = Field(default="")
    embedding_model: str = Field(default="")
    embed_dim: int = Field(default=1024, ge=1, description="D7: must match DB vector dimension at startup")
    enable_rerank: bool = Field(default=True)
    rerank_api_base: str = Field(default="")
    rerank_api_key: str = Field(default="")
    rerank_model: str = Field(default="")


class StorageSettings(_GroupSettings):
    artifact_storage_root: str = Field(
        default="./data/artifacts",
        description="Artifact file root for storage_key addressing (D5, design 4.6)",
    )


class Settings(_FrozenModel):
    shared: SharedSettings
    gateway: GatewaySettings
    diagnosis: DiagnosisSettings
    retrieval: RetrievalSettings
    ingestion: IngestionSettings
    llm: LlmSettings
    storage: StorageSettings

    @model_validator(mode="after")
    def _fail_fast(self) -> Settings:
        failures: list[str] = []
        for group_name, field_names in _PLACEHOLDER_CHECKS:
            group = getattr(self, group_name)
            for field_name in field_names:
                value = str(getattr(group, field_name)).strip().lower()
                if value in PLACEHOLDER_VALUES:
                    failures.append(f"{field_name.upper()} is a placeholder value")
        failures.extend(_conditional_endpoint_failures(self.llm))
        failures.extend(_dsn_failures(self.shared.database_url_cognitive))
        grpc_port = _listen_port(self.gateway.agent_grpc_listen)
        http_port = _listen_port(self.gateway.http_listen)
        if grpc_port == http_port:
            failures.append(
                f"AGENT_GRPC_LISTEN and HTTP_LISTEN must use different ports (both {grpc_port}), see 6.9.2.2"
            )
        if failures:
            raise ValueError("; ".join(failures))
        return self


def _conditional_endpoint_failures(llm: LlmSettings) -> list[str]:
    failures: list[str] = []
    if llm.enable_embedding:
        failures.extend(
            f"{name} is required when ENABLE_EMBEDDING=true (placeholder detected)"
            for name, value in (
                ("EMBEDDING_API_BASE", llm.embedding_api_base),
                ("EMBEDDING_API_KEY", llm.embedding_api_key),
                ("EMBEDDING_MODEL", llm.embedding_model),
            )
            if value.strip().lower() in PLACEHOLDER_VALUES
        )
    if llm.enable_rerank:
        failures.extend(
            f"{name} is required when ENABLE_RERANK=true (placeholder detected)"
            for name, value in (
                ("RERANK_API_BASE", llm.rerank_api_base),
                ("RERANK_API_KEY", llm.rerank_api_key),
                ("RERANK_MODEL", llm.rerank_model),
            )
            if value.strip().lower() in PLACEHOLDER_VALUES
        )
    return failures


def _dsn_failures(dsn: str) -> list[str]:
    parts = urlsplit(dsn.strip())
    if parts.scheme.lower() not in DB_SCHEMES:
        return ["DATABASE_URL_COGNITIVE must use postgresql:// scheme"]
    if (parts.password or "").strip().lower() in PLACEHOLDER_VALUES:
        return ["DATABASE_URL_COGNITIVE password is a placeholder"]
    return []


def _listen_port(listen: str) -> int:
    _, _, port = listen.rpartition(":")
    return int(port)


_GROUPS: tuple[tuple[str, type[BaseSettings]], ...] = (
    ("shared", SharedSettings),
    ("gateway", GatewaySettings),
    ("diagnosis", DiagnosisSettings),
    ("retrieval", RetrievalSettings),
    ("ingestion", IngestionSettings),
    ("llm", LlmSettings),
    ("storage", StorageSettings),
)


def _format_failures(failures: list[str]) -> str:
    return "configuration validation failed (values are never reported):\n" + "\n".join(
        f"  - {failure}" for failure in failures
    )


def _desensitise(group_name: str, exc: ValidationError) -> list[str]:
    lines = []
    for err in exc.errors():
        loc = ".".join(str(part) for part in err.get("loc", ()) or ())
        lines.append(f"{group_name}.{loc}: {err.get('type')}: {err.get('msg')}")
    return lines


def load_settings(env_file: str | Path = _ENV_FILE) -> Settings:
    """构建并校验配置树；任何失败都抛出 ConfigError。

    错误只列出变量名与原因——绝不显示敏感值。
    """
    built: dict[str, BaseSettings] = {}
    failures: list[str] = []
    for group_name, group_cls in _GROUPS:
        try:
            built[group_name] = group_cls(_env_file=env_file)
        except ValidationError as exc:
            failures.extend(_desensitise(group_name, exc))
    if failures:
        raise ConfigError(_format_failures(failures))
    try:
        return Settings(**built)
    except ValidationError as exc:
        raise ConfigError(_format_failures(_desensitise("settings", exc))) from exc