#!/usr/bin/perl
use strict; use warnings;
my $path = shift or die "usage: $0 <file>";
open my $fh, '<:raw', $path or die "open: $!";
local $/; my $src = <$fh>; close $fh;

# 1. Remove opsCleanupDeletedCounts struct + String (moved to executor)
$src =~ s/\ntype opsCleanupDeletedCounts struct \{.*?\}\n\nfunc \(c opsCleanupDeletedCounts\) String\(\) string \{.*?\n\}\n//s
  or die "remove opsCleanupDeletedCounts failed";

# 2. Remove opsCleanupPlan (moved to executor)
$src =~ s/\n\/\/ opsCleanupPlan.*?func opsCleanupPlan\(.*?\}\n//s
  or die "remove opsCleanupPlan failed";

# 3. Remove deleteOldRowsByID (moved to executor)
$src =~ s/\nfunc deleteOldRowsByID\(.*?(?=\n\/\/ truncateOpsTable|\nfunc truncateOpsTable)//s
  or die "remove deleteOldRowsByID failed";

# 4. Remove truncateOpsTable (moved to executor)  
$src =~ s/\n\/\/ truncateOpsTable.*?func truncateOpsTable\(.*?(?=\nfunc isMissingRelationError)//s
  or die "remove truncateOpsTable failed";

# 5. Remove isMissingRelationError (moved to executor)
$src =~ s/\n\/\/ isMissingRelation.*?func isMissingRelationError\(.*?\}\n//s
  or die "remove isMissingRelationError failed";

# 6. Replace runCleanupOnce with table-driven version
my $oldRunOnce = <<'OLD';
func (s *OpsCleanupService) runCleanupOnce(ctx context.Context) (opsCleanupDeletedCounts, error) {
	out := opsCleanupDeletedCounts{}
	if s == nil || s.db == nil || s.cfg == nil {
		return out, nil
	}

	effective := s.snapshotEffective()

	batchSize := 5000

	now := time.Now().UTC()

	// runOne 把"truncate? cutoff? batched delete?"封装到一处，
	// 让三组清理（错误日志类 / 分钟指标 / 小时+日预聚合）调用方只关心表名和列名。
	runOne := func(truncate bool, cutoff time.Time, table, timeCol string, castDate bool) (int64, error) {
		if truncate {
			return truncateOpsTable(ctx, s.db, table)
		}
		return deleteOldRowsByID(ctx, s.db, table, timeCol, cutoff, batchSize, castDate)
	}

	// Error-like tables: error logs / retry attempts / alert events / system logs / cleanup audits.
	if cutoff, truncate, ok := opsCleanupPlan(now, effective.ErrorLogRetentionDays); ok {
		n, err := runOne(truncate, cutoff, "ops_error_logs", "created_at", false)
		if err != nil {
			return out, err
		}
		out.errorLogs = n

		n, err = runOne(truncate, cutoff, "ops_retry_attempts", "created_at", false)
		if err != nil {
			return out, err
		}
		out.retryAttempts = n

		n, err = runOne(truncate, cutoff, "ops_alert_events", "created_at", false)
		if err != nil {
			return out, err
		}
		out.alertEvents = n

		n, err = runOne(truncate, cutoff, "ops_system_logs", "created_at", false)
		if err != nil {
			return out, err
		}
		out.systemLogs = n

		n, err = runOne(truncate, cutoff, "ops_system_log_cleanup_audits", "created_at", false)
		if err != nil {
			return out, err
		}
		out.logAudits = n
	}

	// Minute-level metrics snapshots.
	if cutoff, truncate, ok := opsCleanupPlan(now, effective.MinuteMetricsRetentionDays); ok {
		n, err := runOne(truncate, cutoff, "ops_system_metrics", "created_at", false)
		if err != nil {
			return out, err
		}
		out.systemMetrics = n
	}

	// Pre-aggregation tables (hourly/daily).
	if cutoff, truncate, ok := opsCleanupPlan(now, effective.HourlyMetricsRetentionDays); ok {
		n, err := runOne(truncate, cutoff, "ops_metrics_hourly", "bucket_start", false)
		if err != nil {
			return out, err
		}
		out.hourlyPreagg = n

		n, err = runOne(truncate, cutoff, "ops_metrics_daily", "bucket_date", true)
		if err != nil {
			return out, err
		}
		out.dailyPreagg = n
	}
OLD

my $newRunOnce = <<'NEW';
func (s *OpsCleanupService) runCleanupOnce(ctx context.Context) (opsCleanupDeletedCounts, error) {
	out := opsCleanupDeletedCounts{}
	if s == nil || s.db == nil || s.cfg == nil {
		return out, nil
	}

	effective := s.snapshotEffective()
	now := time.Now().UTC()

	targets := []opsCleanupTarget{
		{effective.ErrorLogRetentionDays, "ops_error_logs", "created_at", false, &out.errorLogs},
		{effective.ErrorLogRetentionDays, "ops_retry_attempts", "created_at", false, &out.retryAttempts},
		{effective.ErrorLogRetentionDays, "ops_alert_events", "created_at", false, &out.alertEvents},
		{effective.ErrorLogRetentionDays, "ops_system_logs", "created_at", false, &out.systemLogs},
		{effective.ErrorLogRetentionDays, "ops_system_log_cleanup_audits", "created_at", false, &out.logAudits},
		{effective.MinuteMetricsRetentionDays, "ops_system_metrics", "created_at", false, &out.systemMetrics},
		{effective.HourlyMetricsRetentionDays, "ops_metrics_hourly", "bucket_start", false, &out.hourlyPreagg},
		{effective.HourlyMetricsRetentionDays, "ops_metrics_daily", "bucket_date", true, &out.dailyPreagg},
	}

	for _, t := range targets {
		cutoff, truncate, ok := opsCleanupPlan(now, t.retentionDays)
		if !ok {
			continue
		}
		n, err := opsCleanupRunOne(ctx, s.db, truncate, cutoff, t.table, t.timeCol, t.castDate, opsCleanupBatchSize)
		if err != nil {
			return out, err
		}
		*t.counter = n
	}
NEW

chomp $oldRunOnce; chomp $newRunOnce;
$src =~ s/\Q$oldRunOnce\E/$newRunOnce/s or die "runCleanupOnce table-driven replace failed";

# 7. Replace magic values with constants
$src =~ s/context\.WithTimeout\(context\.Background\(\), 30\*time\.Minute\)/context.WithTimeout(context.Background(), opsCleanupRunTimeout)/g;
$src =~ s/time\.After\(3 \* time\.Second\)/time.After(opsCleanupCronStopTimeout)/g;
$src =~ s/context\.WithTimeout\(context\.Background\(\), 2\*time\.Second\)/context.WithTimeout(context.Background(), opsCleanupHeartbeatTimeout)/g;
$src =~ s/schedule = "0 2 \* \* \*"/schedule = opsCleanupDefaultSchedule/g;

# 8. Remove unused imports after moving functions
$src =~ s/\t"strings"\n//;

open my $out, '>:raw', $path or die "write: $!";
print $out $src; close $out;
print "cleanup_service.go patched\n";
