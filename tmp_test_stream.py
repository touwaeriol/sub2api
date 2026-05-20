"""临时脚本：测试 sub2api 流式响应结构是否符合规范"""
import json
import sys
import urllib.request
import urllib.error

BASE = "https://clicodeplus.com"
KEY = "sk-4664d76d9f2b2a6ba31f87981eb198c708660deba4b0f3a958cc7074a386cb33"
MODEL = "claude-haiku-4-5-20251001"


def stream_openai():
    """OpenAI 兼容 /v1/chat/completions 流式"""
    print("=" * 60)
    print("【1】OpenAI 兼容 /v1/chat/completions (stream=true)")
    print("=" * 60)
    body = json.dumps({
        "model": MODEL,
        "stream": True,
        "max_tokens": 40,
        "stream_options": {"include_usage": True},
        "messages": [{"role": "user", "content": "只说 hello"}],
    }).encode()
    req = urllib.request.Request(
        f"{BASE}/v1/chat/completions",
        data=body,
        headers={
            "Authorization": f"Bearer {KEY}",
            "Content-Type": "application/json",
        },
        method="POST",
    )
    chunks = []
    with urllib.request.urlopen(req, timeout=30) as resp:
        print(f"HTTP {resp.status}  Content-Type: {resp.headers.get('Content-Type')}")
        for raw in resp:
            line = raw.decode("utf-8", errors="replace").rstrip("\n").rstrip("\r")
            if not line:
                continue
            chunks.append(line)
    print(f"\n共 {len(chunks)} 个非空行\n")
    for i, line in enumerate(chunks):
        if line.startswith("data: "):
            payload = line[6:]
            if payload == "[DONE]":
                print(f"[{i:02d}] data: [DONE]")
                continue
            try:
                obj = json.loads(payload)
                choices = obj.get("choices", [])
                usage = obj.get("usage")
                if choices:
                    delta = choices[0].get("delta", {})
                    fin = choices[0].get("finish_reason")
                    keys = list(delta.keys())
                    preview = delta.get("content")
                    if preview is not None and len(preview) > 40:
                        preview = preview[:40] + "..."
                    print(f"[{i:02d}] delta_keys={keys} content={preview!r} finish={fin}")
                elif usage:
                    print(f"[{i:02d}] usage={usage}")
                else:
                    print(f"[{i:02d}] other={obj}")
            except json.JSONDecodeError:
                print(f"[{i:02d}] !!! 非法 JSON: {payload[:120]}")
        else:
            print(f"[{i:02d}] !!! 不以 'data: ' 开头: {line[:120]}")


def stream_anthropic():
    """Anthropic 原生 /v1/messages 流式"""
    print("\n" + "=" * 60)
    print("【2】Anthropic 原生 /v1/messages (stream=true)")
    print("=" * 60)
    body = json.dumps({
        "model": MODEL,
        "stream": True,
        "max_tokens": 40,
        "messages": [{"role": "user", "content": "只说 hello"}],
    }).encode()
    req = urllib.request.Request(
        f"{BASE}/v1/messages",
        data=body,
        headers={
            "x-api-key": KEY,
            "anthropic-version": "2023-06-01",
            "Content-Type": "application/json",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            print(f"HTTP {resp.status}  Content-Type: {resp.headers.get('Content-Type')}")
            event_name = None
            for raw in resp:
                line = raw.decode("utf-8", errors="replace").rstrip("\n").rstrip("\r")
                if line == "":
                    event_name = None
                    continue
                if line.startswith("event: "):
                    event_name = line[7:]
                    print(f"\nevent: {event_name}")
                elif line.startswith("data: "):
                    payload = line[6:]
                    try:
                        obj = json.loads(payload)
                        print(f"  data: {json.dumps(obj, ensure_ascii=False)[:200]}")
                    except json.JSONDecodeError:
                        print(f"  !!! 非法 JSON data: {payload[:120]}")
                else:
                    print(f"  !!! 未识别行: {line[:120]}")
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        print(f"HTTP {e.code}: {body[:300]}")


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "anthropic":
        stream_anthropic()
    elif len(sys.argv) > 1 and sys.argv[1] == "openai":
        stream_openai()
    else:
        stream_openai()
        stream_anthropic()
