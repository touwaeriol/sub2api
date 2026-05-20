-- Migration 144: introduce Protocol entity
--
-- Protocol 将"协议"从 Group.platform 字符串提升为独立实体：
--   - 每个 Protocol 代表一种网关端点（如 /v1/messages, /v1/chat/completions）
--   - Platform（平台/插件）声明支持哪些 Protocol
--   - Group 通过 protocol_id FK 绑定到具体 Protocol
--   - 账号编辑时只显示其平台支持的 Protocol 下的分组
--
-- 种子数据：根据现有 4 个平台创建默认 Protocol，并回填 groups.protocol_id

-- ============================================================
-- 1. protocols 表
-- ============================================================
CREATE TABLE IF NOT EXISTS protocols (
    id               BIGSERIAL PRIMARY KEY,
    name             VARCHAR(50)  NOT NULL UNIQUE,
    display_name     VARCHAR(100) NOT NULL,
    platform         VARCHAR(50)  NOT NULL,
    gateway_endpoint VARCHAR(200) NOT NULL,
    icon_svg         TEXT,
    theme_color      VARCHAR(20)  NOT NULL DEFAULT '',
    sort_order       INT          NOT NULL DEFAULT 0,
    status           VARCHAR(20)  NOT NULL DEFAULT 'active',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_protocols_platform   ON protocols (platform);
CREATE INDEX IF NOT EXISTS idx_protocols_status     ON protocols (status);
CREATE INDEX IF NOT EXISTS idx_protocols_sort_order ON protocols (sort_order);

-- ============================================================
-- 2. groups 表新增 protocol_id FK
-- ============================================================
ALTER TABLE groups ADD COLUMN IF NOT EXISTS protocol_id BIGINT
    REFERENCES protocols(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_groups_protocol_id ON groups (protocol_id);

-- ============================================================
-- 3. 种子数据：为现有 4 个平台创建默认 Protocol
-- ============================================================
INSERT INTO protocols (name, display_name, platform, gateway_endpoint, theme_color, sort_order)
VALUES
    ('anthropic',   'Anthropic Messages',   'anthropic',   '/v1/messages',          '#D97757', 1),
    ('openai',      'OpenAI Responses',     'openai',      '/v1/responses',         '#10A37F', 2),
    ('gemini',      'Gemini Models',        'gemini',      '/v1beta/models',        '#4285F4', 3),
    ('antigravity', 'Antigravity Messages', 'antigravity', '/v1/messages',          '#8B5CF6', 4)
ON CONFLICT (name) DO NOTHING;

-- ============================================================
-- 4. 回填：将现有 groups 的 platform 映射到对应的 protocol_id
-- ============================================================
UPDATE groups g
SET protocol_id = p.id
FROM protocols p
WHERE g.platform = p.name
  AND g.protocol_id IS NULL;
