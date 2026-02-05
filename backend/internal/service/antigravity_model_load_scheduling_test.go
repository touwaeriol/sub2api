//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ============ 负载比调度算法单元测试 ============

// TestCalculatePriorityWeight 测试权重计算
func TestCalculatePriorityWeight(t *testing.T) {
	tests := []struct {
		name     string
		priority int
		expected float64
	}{
		{
			name:     "priority_0_gets_max_weight",
			priority: 0,
			expected: 10.0,
		},
		{
			name:     "priority_1_gets_max_weight",
			priority: 1,
			expected: 10.0,
		},
		{
			name:     "priority_2_gets_weight_5",
			priority: 2,
			expected: 5.0,
		},
		{
			name:     "priority_5_gets_weight_2",
			priority: 5,
			expected: 2.0,
		},
		{
			name:     "priority_10_gets_weight_1",
			priority: 10,
			expected: 1.0,
		},
		{
			name:     "priority_20_gets_weight_0.5",
			priority: 20,
			expected: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePriorityWeight(tt.priority)
			require.InDelta(t, tt.expected, result, 0.001)
		})
	}
}

// TestCalculateAvgLoadRatio 测试平均负载比计算
func TestCalculateAvgLoadRatio(t *testing.T) {
	tests := []struct {
		name     string
		accounts []accountWithModelLoad
		expected float64
	}{
		{
			name:     "empty_accounts",
			accounts: []accountWithModelLoad{},
			expected: 0,
		},
		{
			name: "all_new_accounts_returns_0",
			accounts: []accountWithModelLoad{
				{account: &Account{Priority: 1}, modelLoadInfo: nil},
				{account: &Account{Priority: 5}, modelLoadInfo: &ModelLoadInfo{CallCount: 0}},
			},
			expected: 0,
		},
		{
			name: "single_account_with_calls",
			accounts: []accountWithModelLoad{
				{account: &Account{Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 10}},
			},
			expected: 1.0, // 10 / 10 = 1
		},
		{
			name: "mixed_new_and_used_accounts",
			accounts: []accountWithModelLoad{
				{account: &Account{Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 10}}, // ratio = 10/10 = 1
				{account: &Account{Priority: 5}, modelLoadInfo: &ModelLoadInfo{CallCount: 4}},  // ratio = 4/2 = 2
				{account: &Account{Priority: 10}, modelLoadInfo: nil},                          // 新账号，不计入平均
			},
			expected: 1.5, // (1 + 2) / 2 = 1.5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateAvgLoadRatio(tt.accounts)
			require.InDelta(t, tt.expected, result, 0.001)
		})
	}
}

// TestSelectByLoadRatio 测试负载比选择
func TestSelectByLoadRatio(t *testing.T) {
	tests := []struct {
		name        string
		accounts    []accountWithModelLoad
		preferOAuth bool
		expectNil   bool
		expectOneOf []int64 // result should be one of these IDs
	}{
		{
			name:      "empty_accounts_returns_nil",
			accounts:  []accountWithModelLoad{},
			expectNil: true,
		},
		{
			name: "single_account",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 0}},
			},
			expectOneOf: []int64{1},
		},
		{
			name: "selects_lowest_load_ratio",
			accounts: []accountWithModelLoad{
				// priority=1, weight=10, calls=10 -> ratio=1.0
				{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 10}},
				// priority=5, weight=2, calls=6 -> ratio=3.0
				{account: &Account{ID: 2, Priority: 5}, modelLoadInfo: &ModelLoadInfo{CallCount: 6}},
				// priority=10, weight=1, calls=5 -> ratio=5.0
				{account: &Account{ID: 3, Priority: 10}, modelLoadInfo: &ModelLoadInfo{CallCount: 5}},
			},
			expectOneOf: []int64{1}, // lowest ratio
		},
		{
			name: "balanced_state_all_same_ratio",
			accounts: []accountWithModelLoad{
				// priority=1, weight=10, calls=100 -> ratio=10.0
				{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 100}},
				// priority=5, weight=2, calls=20 -> ratio=10.0
				{account: &Account{ID: 2, Priority: 5}, modelLoadInfo: &ModelLoadInfo{CallCount: 20}},
				// priority=10, weight=1, calls=10 -> ratio=10.0
				{account: &Account{ID: 3, Priority: 10}, modelLoadInfo: &ModelLoadInfo{CallCount: 10}},
			},
			expectOneOf: []int64{1, 2, 3}, // all have same ratio, any could be selected
		},
		{
			name: "new_accounts_use_avg_ratio",
			accounts: []accountWithModelLoad{
				// priority=1, weight=10, calls=10 -> ratio=1.0
				{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 10}},
				// 新账号，使用平均ratio = 1.0
				{account: &Account{ID: 2, Priority: 1}, modelLoadInfo: nil},
			},
			expectOneOf: []int64{1, 2}, // both have ratio 1.0
		},
		{
			name: "all_new_accounts_random_selection",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: nil},
				{account: &Account{ID: 2, Priority: 5}, modelLoadInfo: nil},
				{account: &Account{ID: 3, Priority: 10}, modelLoadInfo: nil},
			},
			expectOneOf: []int64{1, 2, 3}, // all have ratio 0, any could be selected
		},
		{
			name: "prefer_oauth_when_enabled",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1, Priority: 1, Type: AccountTypeAPIKey}, modelLoadInfo: nil},
				{account: &Account{ID: 2, Priority: 1, Type: AccountTypeOAuth}, modelLoadInfo: nil},
			},
			preferOAuth: true,
			expectOneOf: []int64{2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := selectByLoadRatio(tt.accounts, tt.preferOAuth)
			if tt.expectNil {
				require.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			require.Contains(t, tt.expectOneOf, result.account.ID)
		})
	}
}

// TestSelectByLoadRatio_WeightedDistribution 测试按优先级比例分配
func TestSelectByLoadRatio_WeightedDistribution(t *testing.T) {
	// 验证：优先级 1:5:10 的账号应该承担 10:2:1 的调用量
	// 当调用量为 100:20:10 时，所有账号的负载比应该相等

	accounts := []accountWithModelLoad{
		// priority=1, weight=10, calls=100 -> ratio=10.0
		{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 100}},
		// priority=5, weight=2, calls=20 -> ratio=10.0
		{account: &Account{ID: 2, Priority: 5}, modelLoadInfo: &ModelLoadInfo{CallCount: 20}},
		// priority=10, weight=1, calls=10 -> ratio=10.0
		{account: &Account{ID: 3, Priority: 10}, modelLoadInfo: &ModelLoadInfo{CallCount: 10}},
	}

	// 计算负载比
	avgLoadRatio := calculateAvgLoadRatio(accounts)
	for i := range accounts {
		callCount := int64(0)
		if accounts[i].modelLoadInfo != nil {
			callCount = accounts[i].modelLoadInfo.CallCount
		}
		weight := calculatePriorityWeight(accounts[i].account.Priority)
		if callCount == 0 {
			accounts[i].loadRatio = avgLoadRatio
		} else {
			accounts[i].loadRatio = float64(callCount) / weight
		}
	}

	// 验证所有账号负载比相等（均衡状态）
	require.InDelta(t, 10.0, accounts[0].loadRatio, 0.001)
	require.InDelta(t, 10.0, accounts[1].loadRatio, 0.001)
	require.InDelta(t, 10.0, accounts[2].loadRatio, 0.001)
}

// TestSelectByLoadRatio_HighPriorityGetsMoreCalls 测试高优先级账号承担更多调用
func TestSelectByLoadRatio_HighPriorityGetsMoreCalls(t *testing.T) {
	// 场景：两个账号优先级 1 和 10，初始调用都是 0
	// 高优先级账号应该先被调度

	accounts := []accountWithModelLoad{
		{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 0}},
		{account: &Account{ID: 2, Priority: 10}, modelLoadInfo: &ModelLoadInfo{CallCount: 0}},
	}

	// 两者都是新账号，平均负载比为 0，都可以被选中
	result := selectByLoadRatio(accounts, false)
	require.NotNil(t, result)
	// 由于都是 ratio=0，是随机选择

	// 模拟高优先级账号被调度了 9 次
	accounts = []accountWithModelLoad{
		// priority=1, weight=10, calls=9 -> ratio=0.9
		{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 9}},
		// priority=10, weight=1, calls=0 -> 使用平均ratio=0.9
		{account: &Account{ID: 2, Priority: 10}, modelLoadInfo: nil},
	}

	result = selectByLoadRatio(accounts, false)
	require.NotNil(t, result)
	// 两者负载比相同（0.9），随机选择
	require.Contains(t, []int64{1, 2}, result.account.ID)

	// 模拟高优先级账号被调度了 10 次
	accounts = []accountWithModelLoad{
		// priority=1, weight=10, calls=10 -> ratio=1.0
		{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 10}},
		// priority=10, weight=1, calls=0 -> 使用平均ratio=1.0
		{account: &Account{ID: 2, Priority: 10}, modelLoadInfo: nil},
	}

	result = selectByLoadRatio(accounts, false)
	require.NotNil(t, result)
	// 两者负载比相同（1.0），随机选择
	require.Contains(t, []int64{1, 2}, result.account.ID)

	// 模拟：高优先级调度 10 次，低优先级调度 1 次 -> 均衡状态
	accounts = []accountWithModelLoad{
		// priority=1, weight=10, calls=10 -> ratio=1.0
		{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 10}},
		// priority=10, weight=1, calls=1 -> ratio=1.0
		{account: &Account{ID: 2, Priority: 10}, modelLoadInfo: &ModelLoadInfo{CallCount: 1}},
	}

	result = selectByLoadRatio(accounts, false)
	require.NotNil(t, result)
	// 均衡状态，随机选择
	require.Contains(t, []int64{1, 2}, result.account.ID)
}

// TestSelectByLoadRatio_ColdStartProblem 测试冷启动问题的解决
func TestSelectByLoadRatio_ColdStartProblem(t *testing.T) {
	// 场景：已有账号调用量较大，新加入一个账号
	// 新账号不应该被猛调（使用平均负载比）

	accounts := []accountWithModelLoad{
		// 旧账号：priority=1, weight=10, calls=100 -> ratio=10.0
		{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 100}},
		// 新账号：priority=1, weight=10, calls=0 -> 使用平均ratio=10.0
		{account: &Account{ID: 2, Priority: 1}, modelLoadInfo: nil},
	}

	avgRatio := calculateAvgLoadRatio(accounts)
	require.InDelta(t, 10.0, avgRatio, 0.001) // 平均负载比 = 10

	result := selectByLoadRatio(accounts, false)
	require.NotNil(t, result)
	// 新账号使用平均负载比，两者相同，随机选择
	require.Contains(t, []int64{1, 2}, result.account.ID)
}

// TestFilterOAuthCandidatesFromModelLoad 测试 OAuth 过滤
func TestFilterOAuthCandidatesFromModelLoad(t *testing.T) {
	tests := []struct {
		name        string
		candidates  []*accountWithModelLoad
		expectedIDs []int64
	}{
		{
			name:        "empty_candidates",
			candidates:  []*accountWithModelLoad{},
			expectedIDs: nil,
		},
		{
			name: "no_oauth_accounts",
			candidates: []*accountWithModelLoad{
				{account: &Account{ID: 1, Type: AccountTypeAPIKey}},
				{account: &Account{ID: 2, Type: AccountTypeSetupToken}},
			},
			expectedIDs: nil,
		},
		{
			name: "mixed_accounts",
			candidates: []*accountWithModelLoad{
				{account: &Account{ID: 1, Type: AccountTypeAPIKey}},
				{account: &Account{ID: 2, Type: AccountTypeOAuth}},
				{account: &Account{ID: 3, Type: AccountTypeOAuth}},
			},
			expectedIDs: []int64{2, 3},
		},
		{
			name: "all_oauth_accounts",
			candidates: []*accountWithModelLoad{
				{account: &Account{ID: 1, Type: AccountTypeOAuth}},
				{account: &Account{ID: 2, Type: AccountTypeOAuth}},
			},
			expectedIDs: []int64{1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterOAuthCandidatesFromModelLoad(tt.candidates)
			var resultIDs []int64
			for _, acc := range result {
				resultIDs = append(resultIDs, acc.account.ID)
			}
			require.Equal(t, tt.expectedIDs, resultIDs)
		})
	}
}
