"""Test 1h prompt cache across continuous multi-turn chat."""
import json
import urllib.request
import urllib.error

API_KEY = "sk-4664d76d9f2b2a6ba31f87981eb198c708660deba4b0f3a958cc7074a386cb33"
BASE = "https://clicodeplus.com"
MODEL = "claude-sonnet-4-6"

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

SYSTEM = [
    {"type": "text", "text": LONG_PREFIX,
     "cache_control": {"type": "ephemeral", "ttl": "1h"}}
]


def call(messages, label):
    body = {
        "model": MODEL,
        "max_tokens": 128,
        "system": SYSTEM,
        "messages": messages,
    }
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
        print(f"\n========== {label} ==========")
        print(f"HTTP {e.code}: {e.read().decode('utf-8', errors='replace')}")
        return ""

    usage = data.get("usage", {})
    cc = usage.get("cache_creation") or {}
    content = data.get("content") or []
    text = content[0].get("text", "") if content else ""
    print(f"\n========== {label} ==========")
    print(f"id     : {data.get('id')}")
    print(f"reply  : {text[:180]}")
    print(f"  input_tokens (new, non-cached)  : {usage.get('input_tokens', 0)}")
    print(f"  cache_creation total            : {usage.get('cache_creation_input_tokens', 0)}")
    print(f"    ephemeral_1h_input_tokens     : {cc.get('ephemeral_1h_input_tokens', 0)}")
    print(f"    ephemeral_5m_input_tokens     : {cc.get('ephemeral_5m_input_tokens', 0)}")
    print(f"  cache_read_input_tokens         : {usage.get('cache_read_input_tokens', 0)}")
    print(f"  output_tokens                   : {usage.get('output_tokens', 0)}")
    return text


def main():
    messages = [
        {"role": "user", "content": "用一句话告诉我 RUNBOOK SECTION A 主要讲什么。"}
    ]
    r1 = call(messages, "Turn 1 (start)")

    messages = [
        {"role": "user", "content": "用一句话告诉我 RUNBOOK SECTION A 主要讲什么。"},
        {"role": "assistant", "content": r1 or "(empty)"},
        {"role": "user", "content": "好，那 SECTION B 是讲什么？同样一句话。"},
    ]
    r2 = call(messages, "Turn 2 (system should cache_read)")

    messages = [
        {"role": "user", "content": "用一句话告诉我 RUNBOOK SECTION A 主要讲什么。"},
        {"role": "assistant", "content": r1 or "(empty)"},
        {"role": "user", "content": "好，那 SECTION B 是讲什么？同样一句话。"},
        {"role": "assistant", "content": r2 or "(empty)"},
        {"role": "user", "content": "SECTION C 呢？也用一句话。"},
    ]
    call(messages, "Turn 3 (system should still cache_read)")


if __name__ == "__main__":
    main()
