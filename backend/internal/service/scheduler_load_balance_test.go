//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ─── filterByMinConcurrency ───

func TestFilterByMinConcurrency(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := filterByMinConcurrency(nil)
		require.Empty(t, result)
	})

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 5}},
		}
		result := filterByMinConcurrency(accounts)
		require.Len(t, result, 1)
		require.Equal(t, int64(1), result[0].account.ID)
	})

	t.Run("selects min concurrency only", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 5}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 1}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 3}},
			{account: &Account{ID: 4}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 1}},
		}
		result := filterByMinConcurrency(accounts)
		require.Len(t, result, 2)
		require.Equal(t, int64(2), result[0].account.ID)
		require.Equal(t, int64(4), result[1].account.ID)
	})

	t.Run("all same concurrency returns all", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 3}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 3}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 3}},
		}
		result := filterByMinConcurrency(accounts)
		require.Len(t, result, 3)
	})

	t.Run("zero concurrency preferred", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 5}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
		}
		result := filterByMinConcurrency(accounts)
		require.Len(t, result, 2)
		require.Equal(t, int64(1), result[0].account.ID)
		require.Equal(t, int64(3), result[1].account.ID)
	})
}

// ─── filterByMinCallCount ───

func TestFilterByMinCallCount(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := filterByMinCallCount(nil, nil)
		require.Empty(t, result)
	})

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{1: {CallCount: 10}}
		result := filterByMinCallCount(accounts, loadMap)
		require.Len(t, result, 1)
		require.Equal(t, int64(1), result[0].account.ID)
	})

	t.Run("selects min call count", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 10},
			2: {CallCount: 3},
			3: {CallCount: 7},
		}
		result := filterByMinCallCount(accounts, loadMap)
		require.Len(t, result, 1)
		require.Equal(t, int64(2), result[0].account.ID)
	})

	t.Run("multiple accounts with same min call count", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 5},
			2: {CallCount: 5},
			3: {CallCount: 10},
		}
		result := filterByMinCallCount(accounts, loadMap)
		require.Len(t, result, 2)
		require.Equal(t, int64(1), result[0].account.ID)
		require.Equal(t, int64(2), result[1].account.ID)
	})

	t.Run("new account uses average call count - above average", func(t *testing.T) {
		// 账号 1: 调用 10, 账号 2: 调用 20 → 平均 15
		// 账号 3: 新账号(CallCount=0) → 虚拟值 15
		// 最小应该是账号 1 (10)
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 10},
			2: {CallCount: 20},
			// 3 无记录 → 使用平均值 (10+20)/2 = 15
		}
		result := filterByMinCallCount(accounts, loadMap)
		require.Len(t, result, 1)
		require.Equal(t, int64(1), result[0].account.ID)
	})

	t.Run("new account uses average call count - below average", func(t *testing.T) {
		// 账号 1: 调用 20, 账号 2: 调用 30 → 平均 25
		// 账号 3: 新账号 → 虚拟值 25
		// 最小应该是账号 1 (20)
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 20},
			2: {CallCount: 30},
		}
		result := filterByMinCallCount(accounts, loadMap)
		require.Len(t, result, 1)
		require.Equal(t, int64(1), result[0].account.ID)
	})

	t.Run("new account with equal average", func(t *testing.T) {
		// 账号 1: 调用 10 → 平均 10
		// 账号 2: 新账号 → 虚拟值 10
		// 两个都应该入选
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 10},
		}
		result := filterByMinCallCount(accounts, loadMap)
		require.Len(t, result, 2)
	})

	t.Run("all new accounts - all get zero average", func(t *testing.T) {
		// 全部新账号，没有调用记录，平均值 = 0，所有账号虚拟值 = 0
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{}
		result := filterByMinCallCount(accounts, loadMap)
		require.Len(t, result, 3)
	})

	t.Run("nil model load info for account", func(t *testing.T) {
		// 账号 1: 有记录, 账号 2: modelLoadMap 中无条目(nil)
		// 平均值 = 10, 账号 2 虚拟值 = 10
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 10},
			// 2: nil（不存在于 map 中）
		}
		result := filterByMinCallCount(accounts, loadMap)
		require.Len(t, result, 2) // 10 == 10
	})

	t.Run("CallCount zero treated as new account", func(t *testing.T) {
		// CallCount=0 和无记录等价，都使用平均值
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 5},
			2: {CallCount: 0}, // 显式 0，视为新账号
		}
		result := filterByMinCallCount(accounts, loadMap)
		require.Len(t, result, 2) // 平均=5, 账号2虚拟=5, 两者都是5
	})
}

// ─── filterByMinLastUsed ───

func TestFilterByMinLastUsed(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)
	muchEarlier := now.Add(-2 * time.Hour)

	t.Run("empty slice", func(t *testing.T) {
		result := filterByMinLastUsed(nil, nil)
		require.Empty(t, result)
	})

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{1: {LastUsedAt: now}}
		result := filterByMinLastUsed(accounts, loadMap)
		require.Len(t, result, 1)
	})

	t.Run("selects earliest LastUsedAt", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {LastUsedAt: now},
			2: {LastUsedAt: muchEarlier},
			3: {LastUsedAt: earlier},
		}
		result := filterByMinLastUsed(accounts, loadMap)
		require.Len(t, result, 1)
		require.Equal(t, int64(2), result[0].account.ID)
	})

	t.Run("multiple accounts with same earliest time", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {LastUsedAt: earlier},
			2: {LastUsedAt: now},
			3: {LastUsedAt: earlier},
		}
		result := filterByMinLastUsed(accounts, loadMap)
		require.Len(t, result, 2)
		require.Equal(t, int64(1), result[0].account.ID)
		require.Equal(t, int64(3), result[1].account.ID)
	})

	t.Run("zero time preferred over non-zero", func(t *testing.T) {
		// zero time（从未调度过）视为最早
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {LastUsedAt: now},
			2: {LastUsedAt: time.Time{}}, // 零值
			3: {LastUsedAt: earlier},
		}
		result := filterByMinLastUsed(accounts, loadMap)
		require.Len(t, result, 1)
		require.Equal(t, int64(2), result[0].account.ID)
	})

	t.Run("multiple zero times", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {LastUsedAt: time.Time{}},
			2: {LastUsedAt: now},
			3: {LastUsedAt: time.Time{}},
		}
		result := filterByMinLastUsed(accounts, loadMap)
		require.Len(t, result, 2)
		require.Equal(t, int64(1), result[0].account.ID)
		require.Equal(t, int64(3), result[1].account.ID)
	})

	t.Run("nil model load info treated as zero time", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {LastUsedAt: now},
			// 2: 不在 map 中 → 返回 zero time
		}
		result := filterByMinLastUsed(accounts, loadMap)
		require.Len(t, result, 1)
		require.Equal(t, int64(2), result[0].account.ID)
	})

	t.Run("all same non-zero time returns all", func(t *testing.T) {
		sameTime := now
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {LastUsedAt: sameTime},
			2: {LastUsedAt: sameTime},
		}
		result := filterByMinLastUsed(accounts, loadMap)
		require.Len(t, result, 2)
	})
}

// ─── randomSelectWithOAuthPreference ───

func TestRandomSelectWithOAuthPreference(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := randomSelectWithOAuthPreference(nil, false)
		require.Nil(t, result)
	})

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Type: "session"}, loadInfo: &AccountLoadInfo{}},
		}
		result := randomSelectWithOAuthPreference(accounts, false)
		require.NotNil(t, result)
		require.Equal(t, int64(1), result.account.ID)
	})

	t.Run("no preference - random from all", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, Type: "session"}, loadInfo: &AccountLoadInfo{}},
		}
		validIDs := map[int64]bool{1: true, 2: true, 3: true}
		for i := 0; i < 20; i++ {
			result := randomSelectWithOAuthPreference(accounts, false)
			require.NotNil(t, result)
			require.True(t, validIDs[result.account.ID])
		}
	})

	t.Run("preferOAuth selects from OAuth only", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{}},
		}
		oauthIDs := map[int64]bool{2: true, 3: true}
		for i := 0; i < 20; i++ {
			result := randomSelectWithOAuthPreference(accounts, true)
			require.NotNil(t, result)
			require.True(t, oauthIDs[result.account.ID], "should select OAuth account, got ID=%d", result.account.ID)
		}
	})

	t.Run("preferOAuth fallback when no OAuth", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, Type: "session"}, loadInfo: &AccountLoadInfo{}},
		}
		validIDs := map[int64]bool{1: true, 2: true}
		for i := 0; i < 20; i++ {
			result := randomSelectWithOAuthPreference(accounts, true)
			require.NotNil(t, result)
			require.True(t, validIDs[result.account.ID])
		}
	})
}

// ─── selectByLoadBalance ───

func TestSelectByLoadBalance(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)
	muchEarlier := now.Add(-2 * time.Hour)

	t.Run("empty slice", func(t *testing.T) {
		result := selectByLoadBalance(nil, nil, false)
		require.Nil(t, result)
	})

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 5}},
		}
		result := selectByLoadBalance(accounts, map[int64]*ModelLoadInfo{}, false)
		require.NotNil(t, result)
		require.Equal(t, int64(1), result.account.ID)
	})

	t.Run("nil modelLoadMap falls back to LRU", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			{account: &Account{ID: 2, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 10}},
		}
		// modelLoadMap=nil 时回退到 LRU，应选择 LastUsedAt 最早的
		result := selectByLoadBalance(accounts, nil, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID)
	})

	// ─── 第 1 级：并发量优先 ───

	t.Run("concurrency takes priority over call count", func(t *testing.T) {
		// 账号 1: 并发 5, 调用 1（调用少但并发高）
		// 账号 2: 并发 1, 调用 100（调用多但并发低）
		// 应该选账号 2（并发低优先）
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 5}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 1}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 1, LastUsedAt: muchEarlier},
			2: {CallCount: 100, LastUsedAt: now},
		}
		result := selectByLoadBalance(accounts, loadMap, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID)
	})

	t.Run("concurrency takes priority over LastUsedAt", func(t *testing.T) {
		// 账号 1: 并发 3, LastUsedAt 很久以前
		// 账号 2: 并发 0, LastUsedAt 刚刚
		// 应该选账号 2（并发低优先）
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 3}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 5, LastUsedAt: muchEarlier},
			2: {CallCount: 5, LastUsedAt: now},
		}
		result := selectByLoadBalance(accounts, loadMap, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID)
	})

	// ─── 第 2 级：同并发下按调用次数 ───

	t.Run("same concurrency - selects min call count", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 2}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 2}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 2}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 10, LastUsedAt: muchEarlier},
			2: {CallCount: 3, LastUsedAt: now},
			3: {CallCount: 7, LastUsedAt: earlier},
		}
		result := selectByLoadBalance(accounts, loadMap, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID)
	})

	t.Run("same concurrency - new account cold start protection", func(t *testing.T) {
		// 账号 1: 调用 10, 账号 2: 调用 20 → 平均 15
		// 账号 3: 新账号 → 虚拟值 15
		// 最小是账号 1 (10)
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 10, LastUsedAt: now},
			2: {CallCount: 20, LastUsedAt: now},
			// 3: 新账号
		}
		result := selectByLoadBalance(accounts, loadMap, false)
		require.NotNil(t, result)
		require.Equal(t, int64(1), result.account.ID)
	})

	// ─── 第 3 级：同并发 + 同调用次数下按 LastUsedAt ───

	t.Run("same concurrency and call count - selects earliest LastUsedAt", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 1}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 1}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 1}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 5, LastUsedAt: now},
			2: {CallCount: 5, LastUsedAt: muchEarlier},
			3: {CallCount: 5, LastUsedAt: earlier},
		}
		result := selectByLoadBalance(accounts, loadMap, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID)
	})

	t.Run("same concurrency and call count - zero LastUsedAt preferred", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 5, LastUsedAt: now},
			2: {CallCount: 5, LastUsedAt: time.Time{}}, // 从未调度
		}
		result := selectByLoadBalance(accounts, loadMap, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID)
	})

	// ─── 第 4 级：全部相同时 preferOAuth + 随机 ───

	t.Run("all equal - preferOAuth selects OAuth", func(t *testing.T) {
		sameTime := now
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Type: "session"}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			{account: &Account{ID: 2, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			{account: &Account{ID: 3, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 5, LastUsedAt: sameTime},
			2: {CallCount: 5, LastUsedAt: sameTime},
			3: {CallCount: 5, LastUsedAt: sameTime},
		}
		oauthIDs := map[int64]bool{2: true, 3: true}
		for i := 0; i < 20; i++ {
			result := selectByLoadBalance(accounts, loadMap, true)
			require.NotNil(t, result)
			require.True(t, oauthIDs[result.account.ID], "should select OAuth, got ID=%d", result.account.ID)
		}
	})

	t.Run("all equal no preference - random from all", func(t *testing.T) {
		sameTime := now
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Type: "session"}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			{account: &Account{ID: 2, Type: "session"}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			{account: &Account{ID: 3, Type: "session"}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 5, LastUsedAt: sameTime},
			2: {CallCount: 5, LastUsedAt: sameTime},
			3: {CallCount: 5, LastUsedAt: sameTime},
		}
		validIDs := map[int64]bool{1: true, 2: true, 3: true}
		for i := 0; i < 20; i++ {
			result := selectByLoadBalance(accounts, loadMap, false)
			require.NotNil(t, result)
			require.True(t, validIDs[result.account.ID])
		}
	})

	// ─── 综合集成测试 ───

	t.Run("integration: full 4-level filtering", func(t *testing.T) {
		// 模拟 6 个账号的真实场景
		accounts := []accountWithLoad{
			// 并发 5 → 第 1 级淘汰
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 5}},
			// 并发 0, 调用 20 → 第 2 级淘汰
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			// 并发 0, 调用 3, LastUsedAt = now → 第 3 级淘汰
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			// 并发 0, 调用 3, LastUsedAt = muchEarlier → 最终选中
			{account: &Account{ID: 4}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			// 并发 0, 调用 3, LastUsedAt = earlier → 第 3 级淘汰
			{account: &Account{ID: 5}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			// 并发 3 → 第 1 级淘汰
			{account: &Account{ID: 6}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 3}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 1, LastUsedAt: muchEarlier},
			2: {CallCount: 20, LastUsedAt: muchEarlier},
			3: {CallCount: 3, LastUsedAt: now},
			4: {CallCount: 3, LastUsedAt: muchEarlier},
			5: {CallCount: 3, LastUsedAt: earlier},
			6: {CallCount: 1, LastUsedAt: muchEarlier},
		}
		result := selectByLoadBalance(accounts, loadMap, false)
		require.NotNil(t, result)
		require.Equal(t, int64(4), result.account.ID)
	})

	t.Run("integration: concurrency 90% vs 10% not treated equally", func(t *testing.T) {
		// 核心场景：确认高并发账号不会和低并发账号被同等对待
		// 账号 1: 并发 9（90%满载），调用少，LastUsedAt 最早
		// 账号 2: 并发 1（10%），调用多，LastUsedAt 最近
		// 改造前两者同等对待，改造后账号 2 优先（并发低）
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 9}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 1}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 1, LastUsedAt: muchEarlier},
			2: {CallCount: 50, LastUsedAt: now},
		}
		result := selectByLoadBalance(accounts, loadMap, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID, "should prefer low concurrency over low call count")
	})

	t.Run("integration: tie at all levels - random selection", func(t *testing.T) {
		// 所有账号完全相同
		sameTime := now
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Type: "session"}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 2}},
			{account: &Account{ID: 2, Type: "session"}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 2}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 10, LastUsedAt: sameTime},
			2: {CallCount: 10, LastUsedAt: sameTime},
		}
		seen := map[int64]bool{}
		for i := 0; i < 50; i++ {
			result := selectByLoadBalance(accounts, loadMap, false)
			require.NotNil(t, result)
			seen[result.account.ID] = true
		}
		// 50次随机应该两个都被选中过（极小概率失败 = (0.5)^50）
		require.True(t, seen[1] && seen[2], "random selection should eventually pick both accounts")
	})

	t.Run("integration: new accounts mixed with existing", func(t *testing.T) {
		// 混合新旧账号场景
		// 账号 1: 并发 0, 调用 30
		// 账号 2: 并发 0, 调用 10
		// 账号 3: 并发 0, 新账号 → 虚拟值 = (30+10)/2 = 20
		// 第 1 级: 全部并发=0 → 全通过
		// 第 2 级: 最小调用 = 10 (账号2) → 只剩账号 2
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
		}
		loadMap := map[int64]*ModelLoadInfo{
			1: {CallCount: 30, LastUsedAt: now},
			2: {CallCount: 10, LastUsedAt: now},
			// 3: 新账号
		}
		result := selectByLoadBalance(accounts, loadMap, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID)
	})

	t.Run("integration: cold start - all new accounts with different concurrency", func(t *testing.T) {
		// 全新账号，但并发不同
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 3}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{CurrentConcurrency: 0}},
		}
		loadMap := map[int64]*ModelLoadInfo{} // 全部新账号
		// 第 1 级: 并发 0 → 账号 2, 3
		// 第 2 级: 都是新账号，平均=0，虚拟值=0 → 全通过
		// 第 3 级: 都不在 loadMap → 零值 → 全通过
		// 第 4 级: 随机
		validIDs := map[int64]bool{2: true, 3: true}
		for i := 0; i < 20; i++ {
			result := selectByLoadBalance(accounts, loadMap, false)
			require.NotNil(t, result)
			require.True(t, validIDs[result.account.ID], "should only select concurrency=0 accounts, got ID=%d", result.account.ID)
		}
	})
}
