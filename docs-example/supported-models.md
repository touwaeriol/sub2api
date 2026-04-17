# 支持的模型

本平台通过统一的 API 接口提供多家 AI 模型的访问。

## Claude (Anthropic)

| 模型 | 说明 | 适用场景 |
|------|------|----------|
| `claude-opus-4-20250514` | 最强能力，适合复杂推理 | 复杂编程、长文分析、多步推理 |
| `claude-sonnet-4-20250514` | 性能与速度的平衡 | 日常编码、文档撰写、代码审查 |
| `claude-haiku-4-5-20251001` | 最快速度，适合简单任务 | 快速问答、文本分类、简单生成 |

## GPT (OpenAI)

| 模型 | 说明 | 适用场景 |
|------|------|----------|
| `gpt-4o` | GPT-4 多模态 | 通用对话、图像理解 |
| `gpt-4o-mini` | 轻量版 GPT-4 | 快速任务、成本敏感场景 |

## Gemini (Google)

| 模型 | 说明 | 适用场景 |
|------|------|----------|
| `gemini-2.5-pro` | Google 最强模型 | 长文本、代码生成 |
| `gemini-2.5-flash` | 快速高效 | 日常对话、快速任务 |

> 实际可用的模型取决于管理员配置的渠道。如果某个模型无法使用，请联系管理员。

## API 兼容性

所有模型均通过 OpenAI 兼容格式调用：

```bash
curl https://your-domain.com/v1/chat/completions \
  -H "Authorization: Bearer 你的API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model": "模型名称", "messages": [{"role": "user", "content": "你好"}]}'
```

同时支持 Anthropic 原生格式：

```bash
curl https://your-domain.com/v1/messages \
  -H "x-api-key: 你的API_KEY" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{"model": "claude-sonnet-4-20250514", "max_tokens": 1024, "messages": [{"role": "user", "content": "你好"}]}'
```

## 选择建议

- **日常编码**：`claude-sonnet-4-20250514` — 性价比最高
- **复杂任务**：`claude-opus-4-20250514` — 推理能力最强
- **快速问答**：`claude-haiku-4-5-20251001` 或 `gpt-4o-mini` — 速度快、成本低
- **长文本处理**：`gemini-2.5-pro` — 支持超长上下文
