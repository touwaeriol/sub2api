"""测试 sub2api 的 Anthropic 原生协议 /v1/messages"""
import io
import json
import sys
import time
import urllib.request
import urllib.error

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8")
sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding="utf-8")

BASE = "https://clicodeplus.com"
KEY = "sk-4664d76d9f2b2a6ba31f87981eb198c708660deba4b0f3a958cc7074a386cb33"

MODELS = ["claude-sonnet-4-6", "claude-opus-4-7", "claude-opus-4-6"]

HEADERS = {
    "x-api-key": KEY,
    "anthropic-version": "2023-06-01",
    "Content-Type": "application/json",
}


def post(path, payload, extra_headers=None, timeout=60):
    headers = dict(HEADERS)
    if extra_headers:
        headers.update(extra_headers)
    body = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(f"{BASE}{path}", data=body, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.headers, resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as e:
        return e.code, e.headers, e.read().decode("utf-8", errors="replace")


def post_stream(path, payload, extra_headers=None, timeout=60):
    headers = dict(HEADERS)
    if extra_headers:
        headers.update(extra_headers)
    body = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(f"{BASE}{path}", data=body, headers=headers, method="POST")
    try:
        resp = urllib.request.urlopen(req, timeout=timeout)
    except urllib.error.HTTPError as e:
        return e.code, e.headers, [e.read().decode("utf-8", errors="replace")]
    lines = []
    for raw in resp:
        lines.append(raw.decode("utf-8", errors="replace").rstrip("\n").rstrip("\r"))
    return resp.status, resp.headers, lines


def case_basic_models():
    print("=" * 70)
    print("【1】基础非流式 - 三个模型")
    print("=" * 70)
    for m in MODELS:
        t0 = time.perf_counter()
        code, _, body = post("/v1/messages", {
            "model": m,
            "max_tokens": 50,
            "messages": [{"role": "user", "content": "Reply with exactly: PONG"}],
        })
        dt = (time.perf_counter() - t0) * 1000
        if code != 200:
            print(f"  {m}: HTTP {code} - {body[:200]}")
            continue
        obj = json.loads(body)
        text = obj.get("content", [{}])[0].get("text", "")
        u = obj.get("usage", {})
        print(f"  {m}: {dt:.0f}ms in={u.get('input_tokens')} out={u.get('output_tokens')} text={text!r}")


def case_streaming():
    print("\n" + "=" * 70)
    print("【2】流式响应 - sonnet-4-6")
    print("=" * 70)
    code, _, lines = post_stream("/v1/messages", {
        "model": "claude-sonnet-4-6",
        "max_tokens": 40,
        "stream": True,
        "messages": [{"role": "user", "content": "Count 1 to 3"}],
    })
    if code != 200:
        print(f"HTTP {code}: {lines[0][:200] if lines else ''}")
        return
    events = []
    text_parts = []
    for line in lines:
        if line.startswith("event: "):
            events.append(line[7:])
        elif line.startswith("data: "):
            try:
                obj = json.loads(line[6:])
                if obj.get("type") == "content_block_delta":
                    d = obj.get("delta", {})
                    if d.get("type") == "text_delta":
                        text_parts.append(d.get("text", ""))
            except json.JSONDecodeError:
                pass
    print(f"  事件序列: {events}")
    print(f"  完整文本: {''.join(text_parts)!r}")
    expected = ["message_start", "content_block_start", "content_block_delta",
                "content_block_stop", "message_delta", "message_stop"]
    missing = [e for e in expected if e not in events]
    print(f"  规范完整性: {'✅ 完整' if not missing else '❌ 缺少 ' + str(missing)}")


def case_system_and_multiturn():
    print("\n" + "=" * 70)
    print("【3】system + 多轮对话")
    print("=" * 70)
    code, _, body = post("/v1/messages", {
        "model": "claude-sonnet-4-6",
        "max_tokens": 60,
        "system": "你是一个只说四个字的助手。",
        "messages": [
            {"role": "user", "content": "你好"},
            {"role": "assistant", "content": "你好朋友"},
            {"role": "user", "content": "再见"},
        ],
    })
    if code != 200:
        print(f"  HTTP {code}: {body[:200]}")
        return
    obj = json.loads(body)
    text = obj.get("content", [{}])[0].get("text", "")
    u = obj.get("usage", {})
    print(f"  in={u.get('input_tokens')} out={u.get('output_tokens')} text={text!r}")


def case_tool_use():
    print("\n" + "=" * 70)
    print("【4】Tool use")
    print("=" * 70)
    code, _, body = post("/v1/messages", {
        "model": "claude-sonnet-4-6",
        "max_tokens": 200,
        "tools": [{
            "name": "get_weather",
            "description": "Get the current weather for a location",
            "input_schema": {
                "type": "object",
                "properties": {"location": {"type": "string"}},
                "required": ["location"],
            },
        }],
        "messages": [{"role": "user", "content": "What's the weather in Tokyo?"}],
    })
    if code != 200:
        print(f"  HTTP {code}: {body[:200]}")
        return
    obj = json.loads(body)
    blocks = obj.get("content", [])
    types = [b.get("type") for b in blocks]
    print(f"  stop_reason={obj.get('stop_reason')} block_types={types}")
    for b in blocks:
        if b.get("type") == "tool_use":
            print(f"  tool: name={b.get('name')} input={b.get('input')}")


def case_cache_5m():
    print("\n" + "=" * 70)
    print("【5】Prompt caching (5 分钟默认)")
    print("=" * 70)
    long_text = ("这是一段用于触发缓存的长文本。" * 200)
    payload = {
        "model": "claude-sonnet-4-6",
        "max_tokens": 30,
        "system": [
            {"type": "text", "text": long_text, "cache_control": {"type": "ephemeral"}},
        ],
        "messages": [{"role": "user", "content": "回复 OK"}],
    }
    # 第一次：写入 cache
    code1, _, body1 = post("/v1/messages", payload)
    if code1 != 200:
        print(f"  第一次 HTTP {code1}: {body1[:300]}")
        return
    u1 = json.loads(body1).get("usage", {})
    print(f"  第一次: input={u1.get('input_tokens')} cache_create={u1.get('cache_creation_input_tokens')} cache_read={u1.get('cache_read_input_tokens')}")
    # 第二次：读取 cache
    time.sleep(1)
    code2, _, body2 = post("/v1/messages", payload)
    if code2 != 200:
        print(f"  第二次 HTTP {code2}: {body2[:300]}")
        return
    u2 = json.loads(body2).get("usage", {})
    print(f"  第二次: input={u2.get('input_tokens')} cache_create={u2.get('cache_creation_input_tokens')} cache_read={u2.get('cache_read_input_tokens')}")
    if u2.get("cache_read_input_tokens", 0) > 0:
        print(f"  ✅ 命中 cache（读取 {u2.get('cache_read_input_tokens')} tokens）")
    else:
        print(f"  ❌ 未命中 cache")


def case_cache_1h():
    print("\n" + "=" * 70)
    print("【6】Prompt caching (1 小时 - extended-cache-ttl-2025-04-11)")
    print("=" * 70)
    long_text = ("这是另一段用于 1h cache 的长文本。" * 200)
    payload = {
        "model": "claude-sonnet-4-6",
        "max_tokens": 30,
        "system": [
            {"type": "text", "text": long_text, "cache_control": {"type": "ephemeral", "ttl": "1h"}},
        ],
        "messages": [{"role": "user", "content": "回复 OK"}],
    }
    code, _, body = post("/v1/messages", payload, extra_headers={
        "anthropic-beta": "extended-cache-ttl-2025-04-11",
    })
    if code != 200:
        print(f"  HTTP {code}: {body[:300]}")
        return
    obj = json.loads(body)
    u = obj.get("usage", {})
    cc = u.get("cache_creation", {})
    print(f"  HTTP {code}")
    print(f"  usage: input={u.get('input_tokens')} cache_create={u.get('cache_creation_input_tokens')}")
    print(f"  cache_creation breakdown: 5m={cc.get('ephemeral_5m_input_tokens')} 1h={cc.get('ephemeral_1h_input_tokens')}")
    if cc.get("ephemeral_1h_input_tokens", 0) > 0:
        print(f"  ✅ 1h cache 写入成功")
    else:
        print(f"  ⚠️ 1h cache 未生效")


def case_error_handling():
    print("\n" + "=" * 70)
    print("【7】错误处理")
    print("=" * 70)
    # 无效模型
    code, _, body = post("/v1/messages", {
        "model": "claude-not-exist",
        "max_tokens": 10,
        "messages": [{"role": "user", "content": "x"}],
    })
    print(f"  无效模型: HTTP {code} body={body[:200]}")
    # 缺少 max_tokens
    code, _, body = post("/v1/messages", {
        "model": "claude-sonnet-4-6",
        "messages": [{"role": "user", "content": "x"}],
    })
    print(f"  缺 max_tokens: HTTP {code} body={body[:200]}")


if __name__ == "__main__":
    cases = sys.argv[1:] or ["basic", "stream", "system", "tool", "cache5m", "cache1h", "error"]
    if "basic" in cases:
        case_basic_models()
    if "stream" in cases:
        case_streaming()
    if "system" in cases:
        case_system_and_multiturn()
    if "tool" in cases:
        case_tool_use()
    if "cache5m" in cases:
        case_cache_5m()
    if "cache1h" in cases:
        case_cache_1h()
    if "error" in cases:
        case_error_handling()
