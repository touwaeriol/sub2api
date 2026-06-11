//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type groupCooldownRepoStub struct {
	AccountRepository
	accounts []Account
}

func (s *groupCooldownRepoStub) ListByGroup(ctx context.Context, groupID int64) ([]Account, error) {
	return s.accounts, nil
}

func cooldownAccount(id int64, resetAt *time.Time) Account {
	return Account{
		ID:               id,
		Status:           StatusActive,
		Schedulable:      true,
		RateLimitResetAt: resetAt,
	}
}

func TestGroupRateLimitRecoveryAllCoolingDown(t *testing.T) {
	soon := time.Now().Add(30 * time.Second)
	later := time.Now().Add(5 * time.Minute)
	repo := &groupCooldownRepoStub{accounts: []Account{
		cooldownAccount(1, &soon),
		cooldownAccount(2, &later),
	}}
	groupID := int64(7)

	cooling, wait := groupRateLimitRecovery(context.Background(), repo, &groupID)
	require.True(t, cooling, "全部账号限流中应返回 true")
	require.Greater(t, wait, 20*time.Second, "应取最早恢复时间（约 30s）")
	require.Less(t, wait, time.Minute)
}

func TestGroupRateLimitRecoveryHasAvailableAccount(t *testing.T) {
	soon := time.Now().Add(time.Minute)
	repo := &groupCooldownRepoStub{accounts: []Account{
		cooldownAccount(1, &soon),
		cooldownAccount(2, nil), // 立即可调度：选号失败另有原因，维持 503
	}}
	groupID := int64(7)

	cooling, _ := groupRateLimitRecovery(context.Background(), repo, &groupID)
	require.False(t, cooling)
}

func TestGroupRateLimitRecoveryNilGroupOrEmpty(t *testing.T) {
	repo := &groupCooldownRepoStub{}
	groupID := int64(7)

	cooling, _ := groupRateLimitRecovery(context.Background(), repo, nil)
	require.False(t, cooling, "未分组应返回 false")

	cooling, _ = groupRateLimitRecovery(context.Background(), repo, &groupID)
	require.False(t, cooling, "空分组应返回 false")
}

func TestGroupRateLimitRecoveryIgnoresDisabledAccounts(t *testing.T) {
	soon := time.Now().Add(time.Minute)
	disabled := cooldownAccount(1, &soon)
	disabled.Status = StatusError
	repo := &groupCooldownRepoStub{accounts: []Account{disabled}}
	groupID := int64(7)

	cooling, _ := groupRateLimitRecovery(context.Background(), repo, &groupID)
	require.False(t, cooling, "禁用账号不计入冷却判定")
}
