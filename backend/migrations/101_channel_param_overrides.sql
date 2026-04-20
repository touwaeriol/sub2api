SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE channels ADD COLUMN IF NOT EXISTS param_overrides JSONB NOT NULL DEFAULT '{}'::jsonb;
COMMENT ON COLUMN channels.param_overrides IS '渠道级参数覆盖规则，按平台分区。格式：{"platform": [{"enabled":true,"model_glob":"*","target":"body","action":"set","path":"thinking.budget_tokens","value":1024,"description":"..."}]}';

-- 回滚:
-- ALTER TABLE channels DROP COLUMN param_overrides;
