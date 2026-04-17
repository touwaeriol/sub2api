# 创建 API Key

API Key 用于验证你的 API 请求身份。

## 创建步骤

1. 登录你的账号
2. 进入「控制台」→「API Keys」
3. 点击「创建新 Key」
4. 输入一个便于识别的名称（如 "Claude Code"、"Cursor" 等）
5. 立即复制 Key — 页面关闭后将无法再次查看

## 多 Key 管理

你可以为不同的应用场景创建多个 API Key，方便管理和追踪用量。例如：

- 一个用于 Claude Code
- 一个用于 Cursor
- 一个用于自己的项目

每个 Key 的用量会独立统计，但共享账户余额。

## 安全建议

- **不要**把 API Key 提交到 Git 仓库
- **不要**在公开场合分享你的 Key
- 使用环境变量存储 Key
- 如果 Key 泄露，立即删除并重新创建

```bash
# 推荐：用环境变量存储
export ANTHROPIC_API_KEY="your-key-here"
export OPENAI_API_KEY="your-key-here"
```

## 删除 Key

进入 API Keys 页面，点击对应 Key 旁的删除按钮即可。删除后该 Key 立即失效，使用该 Key 的所有请求都会返回 401 错误。
