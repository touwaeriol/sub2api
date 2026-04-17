# Claude Code 配置

Claude Code 是 Anthropic 官方的 CLI 编程工具。以下是如何将它连接到本平台。

## 配置方法

在终端中设置环境变量即可：

```bash
# 设置 API Key 和 Base URL
export ANTHROPIC_API_KEY="你的API_KEY"
export ANTHROPIC_BASE_URL="https://your-domain.com"
```

设置完成后直接运行 `claude` 命令即可使用。

## 持久化配置

每次打开终端都要重新设置环境变量比较麻烦，可以写入 shell 配置文件：

### macOS / Linux (zsh)

```bash
echo 'export ANTHROPIC_API_KEY="你的API_KEY"' >> ~/.zshrc
echo 'export ANTHROPIC_BASE_URL="https://your-domain.com"' >> ~/.zshrc
source ~/.zshrc
```

### macOS / Linux (bash)

```bash
echo 'export ANTHROPIC_API_KEY="你的API_KEY"' >> ~/.bashrc
echo 'export ANTHROPIC_BASE_URL="https://your-domain.com"' >> ~/.bashrc
source ~/.bashrc
```

### Windows (PowerShell)

```powershell
$env:ANTHROPIC_API_KEY = "你的API_KEY"
$env:ANTHROPIC_BASE_URL = "https://your-domain.com"
```

永久生效需写入 PowerShell 配置：

```powershell
notepad $PROFILE
# 在打开的文件中添加上面两行，保存后重启 PowerShell
```

## 验证连接

配置完成后运行：

```bash
claude "say hello"
```

如果正常返回回答，说明配置成功。

## 常见问题

### 提示 "Invalid API Key"

检查 `ANTHROPIC_API_KEY` 是否正确设置，注意不要有多余的空格或换行。

### 连接超时

检查 `ANTHROPIC_BASE_URL` 是否正确，确保末尾没有多余的 `/`。

### 使用特定模型

Claude Code 默认会自动选择模型。如需指定：

```bash
claude --model claude-sonnet-4-20250514 "your prompt"
```
