import asyncio
import json
import os
import time
import uuid
from datetime import datetime, timezone

import httpx
import pytest

os.environ.setdefault("ALERTFLOW_DATABASE_URL", "postgres://alertflow:alertflow@127.0.0.1:55433/alertflow?sslmode=disable")

from alertflow_agent.artifacts import read_artifact, write_artifact
from alertflow_agent.fd import connect, save_fault_document, search_faults, stable_fd_id
from alertflow_agent.receipts import accept_task, advisory_lock, advisory_unlock, mark_finished, mark_running
from alertflow_agent.schemas import FaultSearchIndex, QueryTimelineInput
from alertflow_agent.textutil import rrf
from alertflow_agent.workflow import OpenAICompatibleModel, ScriptedModel, build_diagnosis_graph, run_archive, run_diagnosis


class FakeGateway:
    def __init__(self, db):
        self.db = db
        self.registered = []

    async def query_timeline(self, state, parsed):
        return [{
            "record_type": "APP_EVENT",
            "record_id": "rec-1",
            "event_time": datetime.now(timezone.utc).isoformat(),
            "received_at": datetime.now(timezone.utc).isoformat(),
            "source": "APP",
            "content": {"event_kind": "HEALTH_STATUS", "health_status": "ABNORMAL", "payload": {"message": "pool exhausted"}},
        }]

    async def search_faults(self, state, query):
        return []

    async def register_artifact(self, state, artifact_type, version, content_type, content_hash):
        self.registered.append(artifact_type)

    async def save_fault(self, task, investigation, report, index):
        return await save_fault_document(
            self.db, task["episode_id"], task["service_id"], task["task_id"], int(task["execution_generation"]),
            investigation, report, index,
        )


@pytest.fixture
async def db():
    conn = await connect()
    async with conn.cursor() as cur:
        await cur.execute("TRUNCATE cognitive.fault_documents, cognitive.episode_artifacts, cognitive.agent_task_receipts")
    await conn.commit()
    yield conn
    await conn.close()


def test_rrf_dedup():
    scores = rrf([["a", "b"], ["b", "c"]])
    assert scores["b"] > scores["a"]


def test_query_timeline_requires_timezone():
    with pytest.raises(ValueError):
        QueryTimelineInput(episode_id="e", start_time=datetime.now(), end_time=datetime.now())


async def test_receipt_is_idempotent(db):
    task = {"task_id": str(uuid.uuid4()), "episode_id": str(uuid.uuid4()), "task_type": "DIAGNOSIS",
            "execution_generation": 1, "input_version_from": 1, "input_version_to": 1}
    first = await accept_task(db, task)
    second = await accept_task(db, task)
    assert first["duplicate"] is False
    assert second["duplicate"] is True
    stale = dict(task, execution_generation=0)
    rejected = await accept_task(db, stale)
    assert rejected["accepted"] is False and rejected["message"] == "stale generation"
    task["execution_generation"] = 2
    third = await accept_task(db, task)
    assert third["duplicate"] is False and third["status"] == "ACCEPTED"


async def test_advisory_lock_blocks_other_connection(db):
    other = await connect()
    key = await advisory_lock(db, "diagnosis:same")
    with pytest.raises(asyncio.TimeoutError):
        await asyncio.wait_for(advisory_lock(other, "diagnosis:same"), timeout=0.3)
    await other.close()
    await advisory_unlock(db, key)
    again = await connect()
    try:
        await asyncio.wait_for(advisory_lock(again, "diagnosis:same"), timeout=1)
    finally:
        await again.close()


async def test_stale_generation_cannot_update_receipt_or_artifact(db):
    episode = str(uuid.uuid4())
    task_id = str(uuid.uuid4())
    task = {"task_id": task_id, "episode_id": episode, "task_type": "DIAGNOSIS", "execution_generation": 1}
    await accept_task(db, task)
    task["execution_generation"] = 2
    await accept_task(db, task)
    await mark_running(db, task_id, 2)
    current = await write_artifact(db, episode, "INVESTIGATION", "MARKDOWN", "generation two", None, task_id, 2, None)
    with pytest.raises(ValueError, match="stale execution generation"):
        await mark_finished(db, task_id, 1, "FAILED", {"error": "stale"}, "SENT")
    with pytest.raises(ValueError, match="stale execution generation"):
        await write_artifact(db, episode, "INVESTIGATION", "MARKDOWN", "generation one", None, task_id, 1, current["version"])
    with pytest.raises(ValueError, match="stale execution generation"):
        await save_fault_document(
            db, episode, str(uuid.uuid4()), task_id, 1, "stale", {},
            FaultSearchIndex(summary="stale", symptoms=[], observations=[], diagnosis=[]),
        )
    saved = await read_artifact(db, episode, "INVESTIGATION")
    assert saved["content_text"] == "generation two"
    assert saved["execution_generation"] == 2


async def test_artifact_version_conflict(db):
    episode = str(uuid.uuid4())
    task_id = str(uuid.uuid4())
    task = {"task_id": task_id, "episode_id": episode, "task_type": "DIAGNOSIS", "execution_generation": 1}
    await accept_task(db, task)
    await mark_running(db, task_id, 1)
    first = await write_artifact(db, episode, "INVESTIGATION", "MARKDOWN", "one", None, task_id, 1, None)
    same = await write_artifact(db, episode, "INVESTIGATION", "MARKDOWN", "one", None, task_id, 1, first["version"])
    assert same["version"] == 1
    second = await write_artifact(db, episode, "INVESTIGATION", "MARKDOWN", "two", None, task_id, 1, 1)
    assert second["version"] == 2
    with pytest.raises(ValueError):
        await write_artifact(db, episode, "INVESTIGATION", "MARKDOWN", "three", None, task_id, 1, 1)


async def test_diagnosis_writes_artifacts_once_per_task(db):
    gateway = FakeGateway(db)
    episode_id = str(uuid.uuid4())
    task = {
        "task_id": str(uuid.uuid4()), "episode_id": episode_id, "task_type": "DIAGNOSIS",
        "execution_generation": 1, "input_version_from": 1, "input_version_to": 1,
        "thread_id": f"diagnosis:{episode_id}", "service_name": "orders",
        "episode_started_at": datetime.now(timezone.utc).isoformat(),
    }
    await accept_task(db, task)
    await mark_running(db, task["task_id"], 1)
    await run_diagnosis(ScriptedModel(), gateway, task)
    await run_diagnosis(ScriptedModel(), gateway, task)
    async with db.cursor() as cur:
        await cur.execute("SELECT content_text FROM cognitive.episode_artifacts WHERE episode_id=%s AND artifact_type='INVESTIGATION'", (episode_id,))
        text = (await cur.fetchone())[0]
    assert text.count(task["task_id"]) == 1
    assert gateway.registered.count("INVESTIGATION") == 2


async def test_archive_search_roundtrip(db):
    gateway = FakeGateway(db)
    episode = str(uuid.uuid4())
    service = str(uuid.uuid4())
    task = {"episode_id": episode, "service_id": service, "task_id": str(uuid.uuid4()), "execution_generation": 1,
            "task_type": "ARCHIVE"}
    await accept_task(db, task)
    await mark_running(db, task["task_id"], 1)
    await write_artifact(db, episode, "INVESTIGATION", "MARKDOWN", "# 故障概况\n\npool exhausted\n", None, task["task_id"], 1, None)
    await write_artifact(db, episode, "DIAGNOSIS_REPORT", "JSON", None, {
        "summary": "连接池耗尽", "observations": ["pool exhausted"], "hypotheses": ["数据库连接池耗尽"],
        "unknowns": [], "next_steps": [],
    }, task["task_id"], 1, None)

    class Model:
        def extract_index(self, inv, report):
            return FaultSearchIndex(summary=report["summary"], symptoms=report["observations"], observations=report["observations"], diagnosis=report["hypotheses"])

    fd_id = await run_archive(Model(), gateway, task)
    assert fd_id == stable_fd_id(episode)
    again = await save_fault_document(
        db, episode, service, task["task_id"], 1, "doc", {"summary": "连接池耗尽"},
        FaultSearchIndex(summary="连接池耗尽", symptoms=["pool exhausted"], observations=["pool exhausted"], diagnosis=["数据库连接池耗尽"]),
    )
    assert again == fd_id
    hits = await search_faults(db, service, "连接池耗尽")
    assert hits and hits[0]["fd_id"] == fd_id
    assert "investigation_content" in hits[0]


    class Gateway:
        db = None

        def __init__(self):
            self.mcp_calls = 0
            self.knowledge_calls = 0
            self.registered = []

        async def invoke_knowledge(self, state):
            self.knowledge_calls += 1
            return {"results": []}

        async def call_mcp(self, state, action):
            self.mcp_calls += 1
            return {"name": action["name"], "text": "ok"}

        async def query_timeline(self, state, parsed):
            return [{"record_type": "APP_EVENT", "record_id": "r1", "event_time": "t", "received_at": "t", "source": "APP", "content": {}}]

        async def search_faults(self, state, query):
            return []

        async def register_artifact(self, state, artifact_type, version, content_type, content_hash):
            self.registered.append(artifact_type)

    class Model:
        def __init__(self, action):
            self.action = action

        def next_action(self, state):
            return self.action

    gateway = Gateway()
    episode = str(uuid.uuid4())
    task = {
        "episode_id": episode, "task_id": str(uuid.uuid4()), "execution_generation": 1,
        "input_version_from": 1, "input_version_to": 1, "service_name": "orders", "service_id": "svc",
        "episode_started_at": datetime.now(timezone.utc).isoformat(), "thread_id": f"diagnosis:{episode}",
        "mcp_tools": [{"name": "safe.read"}], "knowledge_enabled": False,
    }
    result = await run_diagnosis(Model({"type": "call_mcp_tool", "name": "fake.write"}), gateway, task)
    assert result["action"]["type"] == "finish"
    assert gateway.mcp_calls == 0
    assert any("外部动作未执行" in item for item in result["action"]["report"]["observations"])

    task["knowledge_enabled"] = False
    result = await run_diagnosis(Model({"type": "invoke_knowledge_agent"}), gateway, task)
    assert result["action"]["type"] == "finish"
    assert gateway.knowledge_calls == 0
    assert any("外部动作未执行" in item for item in result["action"]["report"]["observations"])


async def test_tool_budget_falls_back_without_external_calls():
    class Gateway:
        db = None

        def __init__(self):
            self.mcp_calls = 0
            self.knowledge_calls = 0
            self.registered = []

        async def query_timeline(self, state, parsed):
            return [{"record_type": "APP_EVENT", "record_id": "r1", "event_time": "t", "received_at": "t", "source": "APP", "content": {}}]

        async def search_faults(self, state, query):
            return [{"fd_id": "fd1", "summary": "historic"}]

        async def invoke_knowledge(self, state):
            self.knowledge_calls += 1
            return {"results": []}

        async def call_mcp(self, state, action):
            self.mcp_calls += 1
            return {"name": action["name"], "text": "side effect"}

        async def register_artifact(self, state, artifact_type, version, content_type, content_hash):
            self.registered.append(artifact_type)

    class Model:
        def next_action(self, state):
            return {"type": "call_mcp_tool", "name": "untrusted.write"}

    gateway = Gateway()
    episode = str(uuid.uuid4())
    task = {
        "episode_id": episode, "task_id": str(uuid.uuid4()), "execution_generation": 1,
        "input_version_from": 1, "input_version_to": 1, "service_name": "orders", "service_id": "svc",
        "episode_started_at": datetime.now(timezone.utc).isoformat(), "thread_id": f"diagnosis:{episode}",
        "knowledge_enabled": True, "mcp_tools": [{"name": "untrusted.write"}],
    }
    from alertflow_agent import workflow as workflow_module
    original_read = workflow_module.read_artifact
    original_write = workflow_module.write_artifact

    async def read(_conn, _episode, _kind):
        return None

    async def write(_conn, episode, kind, content_type, text, data, task_id, generation, expected):
        digest = "0" * 64
        await gateway.register_artifact({}, kind, 1, content_type, digest)
        return {"version": 1, "content_text": text, "content_hash": digest}

    workflow_module.read_artifact = read
    workflow_module.write_artifact = write
    try:
        state = {
            "episode_id": episode, "task_id": task["task_id"], "execution_generation": 1,
            "input_version_from": 1, "input_version_to": 1, "service_name": "orders", "service_id": "svc",
            "episode_started_at": task["episode_started_at"], "monitor_point_ids": [], "app_event_ids": [],
            "tool_rounds": 6, "current_stage": "decide", "knowledge_enabled": True,
            "mcp_tools": task["mcp_tools"], "thread_id": task["thread_id"],
        }
        result = await build_diagnosis_graph(Model(), gateway).ainvoke(state, config={"configurable": {"thread_id": task["thread_id"]}})
    finally:
        workflow_module.read_artifact = original_read
        workflow_module.write_artifact = original_write
    assert result["action"]["type"] == "finish"
    assert gateway.mcp_calls == 0 and gateway.knowledge_calls == 0


async def test_scripted_model_and_async_http():

    seen = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["url"] = str(request.url)
        report = {"summary": "s", "observations": ["o"], "hypotheses": ["h"], "unknowns": [], "next_steps": []}
        body = {"choices": [{"message": {"content": json.dumps({"type": "finish", "report": report})}}]}
        return httpx.Response(200, json=body)

    client = httpx.AsyncClient(transport=httpx.MockTransport(handler), timeout=60)
    model = OpenAICompatibleModel("http://llm/v1", "test-key", "fake", client)
    action = await model.next_action({"episode_id": "e", "service_name": "orders"})
    assert action["type"] == "finish"
    assert seen["url"] == "http://llm/v1/chat/completions"
    assert ScriptedModel().next_action({})["type"] == "query_timeline"
    await client.aclose()
