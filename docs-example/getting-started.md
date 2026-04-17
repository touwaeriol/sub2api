# 快速上手

欢迎使用本平台！只需三步即可开始使用 AI API。

## 第一步：注册账号

在首页点击「登录」，注册一个新账号。支持邮箱注册和 Google 登录。

## 第二步：创建 API Key

登录后进入「控制台」→「API Keys」，点击「创建」生成一个新的 API Key。

> 请妥善保管你的 API Key，创建后只会显示一次。

## 第三步：开始调用

将 API Key 填入你的客户端工具即可开始使用。本平台兼容 OpenAI 和 Anthropic SDK 格式。

### 使用 curl 测试

```bash
curl https://your-domain.com/v1/chat/completions \
  -H "Authorization: Bearer 你的API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

### 使用 Python SDK

```python
from openai import OpenAI

client = OpenAI(
    api_key="你的API_KEY",
    base_url="https://your-domain.com/v1"
)

response = client.chat.completions.create(
    model="claude-sonnet-4-20250514",
    messages=[{"role": "user", "content": "你好"}]
)
print(response.choices[0].message.content)
```

## 下一步

- [创建 API Key](create-api-key) — API Key 管理详解
- [Claude Code 配置](claude-code) — 在 Claude Code 中使用
- [Cursor 配置](cursor) — 在 Cursor 中使用
- [计费说明](billing) — 了解定价和计费方式
