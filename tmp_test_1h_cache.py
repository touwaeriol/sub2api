"""Test 1h prompt cache against https://sub2-cdn.meowai.net via Anthropic protocol."""
import json
import sys
import urllib.request
import urllib.error

API_KEY = "sk-4664d76d9f2b2a6ba31f87981eb198c708660deba4b0f3a958cc7074a386cb33"
BASE = "https://clicodeplus.com"


# Long stable prefix to exceed the 1024-token minimum for Opus models.
LONG_PREFIX = (
    "You are a meticulous senior infrastructure engineer assistant. "
    "Below is an internal runbook excerpt that you must consult when answering. "
    "The runbook covers deployment, observability, incident response, on-call "
    "rotations, capacity planning, and post-mortem authoring. Keep the entire "
    "content in mind even if the user's question is short.\n\n"
    "=== RUNBOOK SECTION A: DEPLOYMENT ===\n"
    + ("Deploy pipelines run on isolated builders with 3 CPUs and 4GB memory. "
       "Each build is fully reproducible via pinned base images and pnpm@9. "
       "Production deploys require a green CI run, a tagged version bump, and "
       "an SSH session to the limited-builder host. Rollbacks are performed by "
       "retagging the previously known-good image and recreating the container.\n") * 24
    + "\n=== RUNBOOK SECTION B: OBSERVABILITY ===\n"
    + ("Structured logs are emitted via slog with mandatory request_id, user_id, "
       "and platform fields. Metrics flow to the Grafana board at "
       "grafana.internal/d/api-latency. Traces are sampled at 5% baseline "
       "but escalate to 100% for endpoints flagged with the slow-tag annotation. "
       "Alerts route through PagerDuty with a 5-minute acknowledgement SLA.\n") * 24
    + "\n=== RUNBOOK SECTION C: INCIDENT RESPONSE ===\n"
    + ("During incidents, the IC opens a war-room channel, captures a timeline, "
       "and delegates investigation streams: traffic, dependencies, data, and "
       "user impact. Mitigations precede root-cause analysis. Customer comms go "
       "out within 30 minutes for Sev1 and 2 hours for Sev2. Postmortems are "
       "drafted within 5 business days and reviewed before publishing.\n") * 24
)


def estimate_tokens(text: str) -> int:
    return max(1, len(text) // 4)

print(f"LONG_PREFIX chars={len(LONG_PREFIX)}, approx_tokens={estimate_tokens(LONG_PREFIX)}")

def build_body(model: str) -> dict:
    return {
        "model": model,
        "max_tokens": 64,
        "system": [
            {
                "type": "text",
                "text": LONG_PREFIX,
                "cache_control": {"type": "ephemeral", "ttl": "1h"},
            }
        ],
        "messages": [
            {"role": "user", "content": "Reply with a single short sentence: cache-test ok."}
        ],
    }


def call(model: str, label: str) -> dict:
    body = build_body(model)
    req = urllib.request.Request(
        f"{BASE}/v1/messages",
        data=json.dumps(body).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "x-api-key": API_KEY,
            "anthropic-version": "2023-06-01",
            "User-Agent": "anthropic-sdk-python/0.34.0",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            data = json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8", errors="replace")
        print(f"\n========== {label} ({model}) ==========")
        print(f"HTTP {e.code}: {err_body}")
        return {}
    except Exception as e:
        print(f"\n========== {label} ({model}) ==========")
        print(f"ERROR: {e}")
        return {}

    usage = data.get("usage", {})
    print(f"\n========== {label} ({model}) ==========")
    print(f"id           : {data.get('id')}")
    print(f"stop_reason  : {data.get('stop_reason')}")
    content = data.get("content") or []
    text = content[0].get("text", "") if content else ""
    print(f"text         : {text[:200]}")
    print(f"usage        : {json.dumps(usage, indent=2, ensure_ascii=False)}")
    cc = usage.get("cache_creation") or {}
    print(f"  ephemeral_1h_input_tokens: {cc.get('ephemeral_1h_input_tokens', 0)}")
    print(f"  ephemeral_5m_input_tokens: {cc.get('ephemeral_5m_input_tokens', 0)}")
    print(f"  cache_creation_input_tokens (total): {usage.get('cache_creation_input_tokens', 0)}")
    print(f"  cache_read_input_tokens   : {usage.get('cache_read_input_tokens', 0)}")
    return data


def main() -> int:
    models = sys.argv[1:] or ["claude-opus-4-7", "claude-opus-4-6", "claude-sonnet-4-6"]
    for m in models:
        # First call: should create 1h cache.
        call(m, f"{m} #1 (create)")
        # Second call: same body, should hit cache (cache_read > 0).
        call(m, f"{m} #2 (read)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
