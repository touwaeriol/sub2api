-- Migration: 111_drop_channel_monitor_deleted_at
-- 清理 110 旧版遗留的 deleted_at 列（如果存在）。
-- 新安装不会有这些列；已有数据库做兼容清理。

DROP INDEX IF EXISTS idx_channel_monitor_histories_deleted_at;
ALTER TABLE channel_monitor_histories
    DROP COLUMN IF EXISTS deleted_at;

DROP INDEX IF EXISTS idx_channel_monitor_daily_rollups_deleted_at;
ALTER TABLE channel_monitor_daily_rollups
    DROP COLUMN IF EXISTS deleted_at;
