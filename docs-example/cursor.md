# Cursor 配置

Cursor 是一款 AI 驱动的代码编辑器。以下是如何将它连接到本平台。

## 配置步骤

1. 打开 Cursor
2. 进入 **Settings** → **Models**
3. 找到 **OpenAI API Key** 部分
4. 填入以下信息：

| 字段 | 值 |
|------|-----|
| API Key | 你的 API Key |
| Base URL | `https://your-domain.com/v1` |

5. 点击 **Save** 保存
6. 在模型列表中选择你想使用的模型

## 支持的模型

在 Cursor 中可以使用以下模型（取决于管理员配置）：

- `claude-sonnet-4-20250514` — 推荐日常编码使用
- `claude-opus-4-20250514` — 复杂任务
- `gpt-4o` — OpenAI GPT-4
- `gemini-2.5-pro` — Google Gemini

## 使用技巧

### Composer 模式

在 Cursor 中按 `Cmd+I`（macOS）或 `Ctrl+I`（Windows/Linux）打开 Composer，可以用自然语言描述需求，AI 会直接生成或修改代码。

### Chat 模式

按 `Cmd+L` 或 `Ctrl+L` 打开 Chat 面板，适合提问和讨论代码逻辑。

### 选中代码提问

选中一段代码后按 `Cmd+L`，可以针对选中的代码提问或要求修改。

## 常见问题

### Cursor 提示 API Key 无效

确认你的 Base URL 格式正确，末尾需要有 `/v1`：

```
# 正确
https://your-domain.com/v1

# 错误
https://your-domain.com
https://your-domain.com/v1/
```

### 响应速度慢

可以尝试切换到更快的模型，如 `claude-sonnet-4-20250514` 或 `gpt-4o-mini`。
