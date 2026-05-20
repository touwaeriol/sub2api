"""测试 Gemini 原生协议生图（gemini-3.1-flash-image）"""
import base64
import io
import json
import sys
import time
import urllib.request
import urllib.error

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8")
sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding="utf-8")

BASE = "https://clicodeplus.com"
KEY = "sk-d89de325ecd95dbf4b6840c7d1615da416b85d2417867fec5fc4e1747d0ee441"
MODEL = "gemini-3.1-flash-image"


def call_generate(prompt: str, response_modalities=None):
    url = f"{BASE}/v1beta/models/{MODEL}:generateContent"
    payload = {
        "contents": [{"role": "user", "parts": [{"text": prompt}]}],
    }
    if response_modalities:
        payload["generationConfig"] = {"responseModalities": response_modalities}
    body = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=body,
        headers={
            "x-goog-api-key": KEY,
            "Content-Type": "application/json",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            return resp.status, resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", errors="replace")


def summarize(label: str, code: int, body: str, save_prefix: str | None = None):
    print(f"\n=== {label} ===")
    print(f"HTTP {code}")
    if code != 200:
        print(f"  body: {body[:500]}")
        return
    obj = json.loads(body)
    cands = obj.get("candidates", [])
    if not cands:
        print(f"  无 candidates: {body[:500]}")
        return
    parts = cands[0].get("content", {}).get("parts", [])
    image_idx = 0
    for i, p in enumerate(parts):
        if "text" in p:
            t = p["text"]
            print(f"  [{i}] text({len(t)}字符): {t[:120]!r}")
        elif "inlineData" in p or "inline_data" in p:
            inline = p.get("inlineData") or p.get("inline_data")
            mime = inline.get("mimeType") or inline.get("mime_type")
            data = inline.get("data", "")
            try:
                decoded = base64.b64decode(data)
                size = len(decoded)
            except Exception:
                size = -1
            print(f"  [{i}] image: mime={mime} base64_len={len(data)} decoded_bytes={size}")
            if save_prefix and size > 0:
                ext = "png" if "png" in (mime or "") else ("jpg" if "jpeg" in (mime or "") else "bin")
                fn = f"{save_prefix}_{image_idx}.{ext}"
                with open(fn, "wb") as f:
                    f.write(decoded)
                print(f"       已保存: {fn}")
                image_idx += 1
        else:
            print(f"  [{i}] 未知 part: keys={list(p.keys())}")
    usage = obj.get("usageMetadata") or {}
    if usage:
        print(f"  usage: {usage}")
    finish = cands[0].get("finishReason")
    print(f"  finishReason: {finish}")


def case_text_to_image():
    prompt = "Generate an image of a cute orange cat sitting on a stack of programming books, photorealistic style."
    t0 = time.perf_counter()
    code, body = call_generate(prompt, response_modalities=["IMAGE"])
    dt = (time.perf_counter() - t0) * 1000
    summarize(f"【1】文生图（responseModalities=IMAGE） {dt:.0f}ms", code, body, save_prefix="tmp_gemini_img1")


def case_text_and_image():
    prompt = "画一只蓝色的小鸟，并简短描述这只鸟。"
    t0 = time.perf_counter()
    code, body = call_generate(prompt, response_modalities=["TEXT", "IMAGE"])
    dt = (time.perf_counter() - t0) * 1000
    summarize(f"【2】图文混合（TEXT+IMAGE） {dt:.0f}ms", code, body, save_prefix="tmp_gemini_img2")


def case_no_modalities():
    prompt = "draw a small red apple on a white background"
    t0 = time.perf_counter()
    code, body = call_generate(prompt)
    dt = (time.perf_counter() - t0) * 1000
    summarize(f"【3】不指定 responseModalities {dt:.0f}ms", code, body, save_prefix="tmp_gemini_img3")


def case_list_models():
    print("\n=== 【0】列出模型 ===")
    req = urllib.request.Request(
        f"{BASE}/v1beta/models",
        headers={"x-goog-api-key": KEY},
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            models = [m.get("name") for m in data.get("models", [])]
            target_hits = [m for m in models if "image" in (m or "").lower() or "flash" in (m or "").lower()]
            print(f"  共 {len(models)} 个模型")
            print(f"  含 image/flash 的: {target_hits[:10]}")
    except urllib.error.HTTPError as e:
        print(f"  HTTP {e.code}: {e.read().decode('utf-8')[:300]}")


if __name__ == "__main__":
    case_list_models()
    case_text_to_image()
    case_text_and_image()
    case_no_modalities()
