package service

import (
	"context"
	"time"
)

// groupRateLimitRecovery 判断分组内是否"所有启用的账号都处于限流/过载/临时停调冷却中"，
// 是则返回距最早恢复的等待时长。供 handler 在选号失败时以 429 + Retry-After 表达上游
// 限流语义（而非 503），让下游网关/SDK 能正确退避重试。
//
// 返回 (false, 0) 的情况：未分组、查询失败、分组内无启用账号、存在立即可用账号
// （选号失败另有原因，维持 503 语义）、或不可用原因不是冷却（禁用/过期/配额）。
func groupRateLimitRecovery(ctx context.Context, repo AccountRepository, groupID *int64) (bool, time.Duration) {
	if repo == nil || groupID == nil {
		return false, 0
	}
	accounts, err := repo.ListByGroup(ctx, *groupID)
	if err != nil {
		return false, 0
	}

	now := time.Now()
	var earliest time.Time
	coolingDown := 0
	for i := range accounts {
		acc := &accounts[i]
		if !acc.IsActive() || !acc.Schedulable {
			continue
		}
		recovery := accountCooldownRecoveryTime(acc, now)
		if recovery.IsZero() {
			if acc.IsSchedulable() {
				// 存在立即可调度的账号：选号失败另有原因（模型不支持等），维持 503
				return false, 0
			}
			continue // 非冷却原因不可用（过期/配额），不计入
		}
		coolingDown++
		if earliest.IsZero() || recovery.Before(earliest) {
			earliest = recovery
		}
	}
	if coolingDown == 0 {
		return false, 0
	}
	return true, time.Until(earliest)
}

// accountCooldownRecoveryTime 返回账号冷却（限流/过载/临时停调）的恢复时间；无冷却返回零值。
// 账号需等所有冷却窗口结束才可调度，取各窗口的最大值。
func accountCooldownRecoveryTime(a *Account, now time.Time) time.Time {
	var recovery time.Time
	for _, t := range []*time.Time{a.OverloadUntil, a.RateLimitResetAt, a.TempUnschedulableUntil} {
		if t != nil && now.Before(*t) && t.After(recovery) {
			recovery = *t
		}
	}
	return recovery
}

// GroupRateLimitRecovery 见 groupRateLimitRecovery。
func (s *GatewayService) GroupRateLimitRecovery(ctx context.Context, groupID *int64) (bool, time.Duration) {
	if s == nil {
		return false, 0
	}
	return groupRateLimitRecovery(ctx, s.accountRepo, groupID)
}

// GroupRateLimitRecovery 见 groupRateLimitRecovery。
func (s *OpenAIGatewayService) GroupRateLimitRecovery(ctx context.Context, groupID *int64) (bool, time.Duration) {
	if s == nil {
		return false, 0
	}
	return groupRateLimitRecovery(ctx, s.accountRepo, groupID)
}
