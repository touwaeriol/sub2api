-- 统一 Antigravity 模型白名单/映射字段
-- 将原有的 model_whitelist 逻辑合并到 model_mapping 中
-- model_mapping 的 key 作为白名单，value 作为映射目标（精确匹配，无通配符）

-- 为未配置 model_mapping 的 Antigravity OAuth 账号填充默认映射
-- 基于官方 API 返回的模型列表，只支持 Claude 4.5+ 和 Gemini 2.5+
-- Claude 4 及更早版本不支持

UPDATE accounts
SET credentials = credentials || '{
  "model_mapping": {
    "claude-opus-4-6": "claude-opus-4-6",
    "claude-opus-4-5-thinking": "claude-opus-4-5-thinking",
    "claude-opus-4-5-20251101": "claude-opus-4-5-thinking",
    "claude-sonnet-4-5": "claude-sonnet-4-5",
    "claude-sonnet-4-5-thinking": "claude-sonnet-4-5-thinking",
    "claude-sonnet-4-5-20250929": "claude-sonnet-4-5",
    "claude-haiku-4-5": "claude-sonnet-4-5",
    "claude-haiku-4-5-20251001": "claude-sonnet-4-5",
    "gemini-2.5-flash": "gemini-2.5-flash",
    "gemini-2.5-flash-lite": "gemini-2.5-flash-lite",
    "gemini-2.5-flash-thinking": "gemini-2.5-flash-thinking",
    "gemini-2.5-pro": "gemini-2.5-pro",
    "gemini-3-flash": "gemini-3-flash",
    "gemini-3-flash-preview": "gemini-3-flash",
    "gemini-3-pro-high": "gemini-3-pro-high",
    "gemini-3-pro-low": "gemini-3-pro-low",
    "gemini-3-pro-image": "gemini-3-pro-image",
    "gemini-3-pro-preview": "gemini-3-pro-high",
    "gemini-3-pro-image-preview": "gemini-3-pro-image",
    "gpt-oss-120b-medium": "gpt-oss-120b-medium",
    "tab_flash_lite_preview": "tab_flash_lite_preview"
  }
}'::jsonb
WHERE platform = 'antigravity'
  AND type = 'oauth'
  AND deleted_at IS NULL
  AND NOT (credentials ? 'model_mapping');

-- 对于已配置 model_whitelist 但未配置 model_mapping 的账号，
-- 将 model_whitelist 转换为 model_mapping（精确匹配）
-- 注意：这种转换保持精确匹配语义

UPDATE accounts
SET credentials = credentials - 'model_whitelist' ||
  jsonb_build_object('model_mapping',
    (SELECT jsonb_object_agg(elem, elem)
     FROM jsonb_array_elements_text(credentials->'model_whitelist') AS elem)
  )
WHERE platform = 'antigravity'
  AND type = 'oauth'
  AND deleted_at IS NULL
  AND credentials ? 'model_whitelist'
  AND NOT (credentials ? 'model_mapping');

-- 对于已同时配置 model_whitelist 和 model_mapping 的账号，
-- 直接删除 model_whitelist（model_mapping 优先）

UPDATE accounts
SET credentials = credentials - 'model_whitelist'
WHERE platform = 'antigravity'
  AND type = 'oauth'
  AND deleted_at IS NULL
  AND credentials ? 'model_whitelist'
  AND credentials ? 'model_mapping';
