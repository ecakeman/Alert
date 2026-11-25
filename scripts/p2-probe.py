import json
import sys
import time
import urllib.error
import urllib.request

REGISTRY = "http://127.0.0.1:8081"
BFF = "http://127.0.0.1:8080"


def call(base, method, path, payload=None):
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(base + path, data=data, method=method,
                                 headers={"Content-Type": "application/json"})
    try:
        resp = urllib.request.urlopen(req)
        body = resp.read().decode()
        return resp.status, json.loads(body) if body else None
    except urllib.error.HTTPError as err:
        body = err.read().decode()
        try:
            return err.code, json.loads(body)
        except json.JSONDecodeError:
            return err.code, body


checks = []


def expect(name, actual, want):
    checks.append({"name": name, "actual": actual, "expected": want, "ok": actual == want})


key = f"p2-gate-{int(time.time())}"
status, created = call(REGISTRY, "POST", "/internal/services", {
    "service_key": key, "name": "P2 Gate", "description": "gate",
    "authority_producer_id": "gate",
    "targets": [{
        "target_key": "http", "target_name": "HTTP", "target_type": "HTTP",
        "address": "http://127.0.0.1:19030/healthz", "interval_ms": 15000,
        "timeout_ms": 3000, "failure_threshold": 2, "recovery_threshold": 2,
        "is_required": True, "enabled": True,
    }],
})
expect("create", status, 201)
sid = created["service_id"]
tid = created["targets"][0]["target_id"]

expect("get", call(REGISTRY, "GET", f"/internal/services/{sid}")[0], 200)
expect("patch", call(REGISTRY, "PATCH", f"/internal/services/{sid}", {"description": "patched"})[0], 200)
expect("targets", call(REGISTRY, "GET", f"/internal/services/{sid}/targets")[0], 200)
expect("target-patch", call(REGISTRY, "PATCH", f"/internal/targets/{tid}", {"enabled": False})[0], 200)
expect("source-states", call(REGISTRY, "GET", f"/internal/services/{sid}/source-states")[0], 200)
expect("stats", call(REGISTRY, "GET", "/internal/stats")[0], 200)
expect("not-found", call(REGISTRY, "GET", "/internal/services/00000000-0000-0000-0000-000000000000")[0], 404)
expect("conflict", call(REGISTRY, "POST", "/internal/services", {
    "service_key": key, "name": "dup", "description": "", "authority_producer_id": "x", "targets": [],
})[0], 409)
ids = ",".join(f"00000000-0000-0000-0000-{i:012d}" for i in range(201))
expect("ids-limit", call(REGISTRY, "GET", f"/internal/services?ids={ids}")[0], 422)
expect("delete-target", call(REGISTRY, "DELETE", f"/internal/targets/{tid}")[0], 204)
expect("delete", call(REGISTRY, "DELETE", f"/internal/services/{sid}")[0], 204)
expect("soft-deleted", call(REGISTRY, "GET", f"/internal/services/{sid}")[0], 404)

# 经 BFF 的路由 2/4/7 两次响应一致（registry 域可直通部分）
def stable(path):
    first = call(BFF, "GET", path)[1]
    second = call(BFF, "GET", path)[1]
    return json.dumps(first, sort_keys=True) == json.dumps(second, sort_keys=True)


expect("bff-list-stable", stable("/api/v1/services?limit=5"), True)
expect("bff-targets-stable", stable("/api/v1/services/11111111-1111-1111-1111-111111111111/targets"), True)

ok = all(item["ok"] for item in checks)
print(json.dumps({"checks": checks, "ok": ok}, ensure_ascii=False))
raise SystemExit(0 if ok else 1)