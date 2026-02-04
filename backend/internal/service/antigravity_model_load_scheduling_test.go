//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ============ 模型负载调度算法单元测试 ============

// TestCalculateEffectivePriority 测试有效优先级计算
func TestCalculateEffectivePriority(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		priority   int
		lastUsedAt time.Time
		expected   int
	}{
		{
			name:       "never_used_returns_1",
			priority:   10,
			lastUsedAt: time.Time{}, // 零值表示从未使用
			expected:   1,
		},
		{
			name:       "just_used_returns_original",
			priority:   10,
			lastUsedAt: now,
			expected:   10,
		},
		{
			name:       "1_minute_ago_decays_20_percent",
			priority:   10,
			lastUsedAt: now.Add(-1 * time.Minute),
			expected:   8, // ceil(10 * 0.8) = 8
		},
		{
			name:       "5_minutes_ago",
			priority:   10,
			lastUsedAt: now.Add(-5 * time.Minute),
			expected:   4, // ceil(10 * 0.8^5) = ceil(10 * 0.328) = ceil(3.28) = 4
		},
		{
			name:       "10_minutes_ago_near_minimum",
			priority:   10,
			lastUsedAt: now.Add(-10 * time.Minute),
			expected:   2, // ceil(10 * 0.8^10) = ceil(10 * 0.107) = ceil(1.07) = 2
		},
		{
			name:       "15_minutes_ago_at_minimum",
			priority:   10,
			lastUsedAt: now.Add(-15 * time.Minute),
			expected:   1, // ceil(10 * 0.8^15) = ceil(10 * 0.035) = ceil(0.35) = 1
		},
		{
			name:       "high_priority_decays_slower",
			priority:   100,
			lastUsedAt: now.Add(-10 * time.Minute),
			expected:   11, // ceil(100 * 0.8^10) = ceil(100 * 0.107) = ceil(10.7) = 11
		},
		{
			name:       "priority_1_stays_1",
			priority:   1,
			lastUsedAt: now.Add(-5 * time.Minute),
			expected:   1, // ceil(1 * 0.8^5) = ceil(0.328) = 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateEffectivePriority(tt.priority, tt.lastUsedAt)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateLoadScores(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		accounts []accountWithModelLoad
		expected []int64
	}{
		{
			name:     "empty_accounts",
			accounts: []accountWithModelLoad{},
			expected: []int64{},
		},
		{
			name: "never_used_account_gets_priority_1",
			accounts: []accountWithModelLoad{
				{account: &Account{Priority: 10}, modelLoadInfo: nil}, // 从未使用 -> 有效优先级 1
			},
			expected: []int64{1}, // 1 * (1 + 0) = 1
		},
		{
			name: "recently_used_account_keeps_priority",
			accounts: []accountWithModelLoad{
				{account: &Account{Priority: 2}, modelLoadInfo: &ModelLoadInfo{CallCount: 5, LastUsedAt: now}},
			},
			expected: []int64{12}, // 2 * (1 + 5) = 12
		},
		{
			name: "priority_zero_always_zero",
			accounts: []accountWithModelLoad{
				{account: &Account{Priority: 0}, modelLoadInfo: &ModelLoadInfo{CallCount: 100, LastUsedAt: now}},
			},
			expected: []int64{0}, // 0 * (1 + 100) = 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calculateLoadScores(tt.accounts)
			for i, acc := range tt.accounts {
				require.Equal(t, tt.expected[i], acc.loadScore, "account %d", i)
			}
		})
	}
}

func TestFilterByMinLoadScore(t *testing.T) {
	tests := []struct {
		name        string
		accounts    []accountWithModelLoad
		expectedIDs []int64
	}{
		{
			name:        "empty_accounts",
			accounts:    []accountWithModelLoad{},
			expectedIDs: nil,
		},
		{
			name: "single_account",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1}, loadScore: 10},
			},
			expectedIDs: []int64{1},
		},
		{
			name: "multiple_accounts_different_scores",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1}, loadScore: 10},
				{account: &Account{ID: 2}, loadScore: 5},
				{account: &Account{ID: 3}, loadScore: 15},
			},
			expectedIDs: []int64{2},
		},
		{
			name: "multiple_accounts_same_min_score",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1}, loadScore: 5},
				{account: &Account{ID: 2}, loadScore: 5},
				{account: &Account{ID: 3}, loadScore: 10},
			},
			expectedIDs: []int64{1, 2},
		},
		{
			name: "all_accounts_same_score",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1}, loadScore: 3},
				{account: &Account{ID: 2}, loadScore: 3},
				{account: &Account{ID: 3}, loadScore: 3},
			},
			expectedIDs: []int64{1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterByMinLoadScore(tt.accounts)
			var resultIDs []int64
			for _, acc := range result {
				resultIDs = append(resultIDs, acc.account.ID)
			}
			require.Equal(t, tt.expectedIDs, resultIDs)
		})
	}
}

func TestFilterByMinPriorityFromModelLoad(t *testing.T) {
	tests := []struct {
		name        string
		accounts    []accountWithModelLoad
		expectedIDs []int64
	}{
		{
			name:        "empty_accounts",
			accounts:    []accountWithModelLoad{},
			expectedIDs: nil,
		},
		{
			name: "single_account",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1, Priority: 5}},
			},
			expectedIDs: []int64{1},
		},
		{
			name: "multiple_accounts_different_priorities",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1, Priority: 3}},
				{account: &Account{ID: 2, Priority: 1}},
				{account: &Account{ID: 3, Priority: 2}},
			},
			expectedIDs: []int64{2},
		},
		{
			name: "multiple_accounts_same_min_priority",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1, Priority: 1}},
				{account: &Account{ID: 2, Priority: 1}},
				{account: &Account{ID: 3, Priority: 2}},
			},
			expectedIDs: []int64{1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterByMinPriorityFromModelLoad(tt.accounts)
			var resultIDs []int64
			for _, acc := range result {
				resultIDs = append(resultIDs, acc.account.ID)
			}
			require.Equal(t, tt.expectedIDs, resultIDs)
		})
	}
}

func TestCollectModelLRUCandidates(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)
	earliest := now.Add(-2 * time.Hour)

	tests := []struct {
		name        string
		accounts    []accountWithModelLoad
		expectedIDs []int64
	}{
		{
			name: "all_zero_time_accounts",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1}, modelLoadInfo: nil},
				{account: &Account{ID: 2}, modelLoadInfo: &ModelLoadInfo{LastUsedAt: time.Time{}}},
			},
			expectedIDs: []int64{1, 2},
		},
		{
			name: "mixed_zero_and_non_zero",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1}, modelLoadInfo: &ModelLoadInfo{LastUsedAt: now}},
				{account: &Account{ID: 2}, modelLoadInfo: nil}, // zero time - should be selected
			},
			expectedIDs: []int64{2},
		},
		{
			name: "all_non_zero_different_times",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1}, modelLoadInfo: &ModelLoadInfo{LastUsedAt: now}},
				{account: &Account{ID: 2}, modelLoadInfo: &ModelLoadInfo{LastUsedAt: earliest}}, // earliest
				{account: &Account{ID: 3}, modelLoadInfo: &ModelLoadInfo{LastUsedAt: earlier}},
			},
			expectedIDs: []int64{2},
		},
		{
			name: "multiple_accounts_same_earliest_time",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1}, modelLoadInfo: &ModelLoadInfo{LastUsedAt: earliest}},
				{account: &Account{ID: 2}, modelLoadInfo: &ModelLoadInfo{LastUsedAt: earliest}},
				{account: &Account{ID: 3}, modelLoadInfo: &ModelLoadInfo{LastUsedAt: now}},
			},
			expectedIDs: []int64{1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collectModelLRUCandidates(tt.accounts)
			var resultIDs []int64
			for _, acc := range result {
				resultIDs = append(resultIDs, acc.account.ID)
			}
			require.ElementsMatch(t, tt.expectedIDs, resultIDs)
		})
	}
}

func TestSelectByModelLoad(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)

	tests := []struct {
		name        string
		accounts    []accountWithModelLoad
		preferOAuth bool
		// We can't predict exact result due to random selection in some cases,
		// so we test expected properties
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
				{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 0, LastUsedAt: now}},
			},
			expectOneOf: []int64{1},
		},
		{
			name: "selects_lowest_load_score",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1, Priority: 2}, modelLoadInfo: &ModelLoadInfo{CallCount: 5, LastUsedAt: now}}, // score: 2*(1+5)=12
				{account: &Account{ID: 2, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 2, LastUsedAt: now}}, // score: 1*(1+2)=3  <- lowest
				{account: &Account{ID: 3, Priority: 3}, modelLoadInfo: &ModelLoadInfo{CallCount: 0, LastUsedAt: now}}, // score: 3*(1+0)=3  <- tied
			},
			expectOneOf: []int64{2, 3}, // both have score 3, either could be selected
		},
		{
			name: "tie_break_by_priority",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1, Priority: 2}, modelLoadInfo: &ModelLoadInfo{CallCount: 1, LastUsedAt: now}}, // score: 2*(1+1)=4
				{account: &Account{ID: 2, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 3, LastUsedAt: now}}, // score: 1*(1+3)=4, priority 1
				{account: &Account{ID: 3, Priority: 3}, modelLoadInfo: &ModelLoadInfo{CallCount: 0, LastUsedAt: now}}, // score: 3*(1+0)=3 <- lowest score, selected
			},
			expectOneOf: []int64{3},
		},
		{
			name: "tie_break_by_lru",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 0, LastUsedAt: now}},     // score: 1, priority: 1, used now
				{account: &Account{ID: 2, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 0, LastUsedAt: earlier}}, // score: 1, priority: 1, used earlier <- should be selected
			},
			expectOneOf: []int64{2},
		},
		{
			name: "prefer_never_used",
			accounts: []accountWithModelLoad{
				{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 0, LastUsedAt: now}},
				{account: &Account{ID: 2, Priority: 1}, modelLoadInfo: nil}, // never used <- should be selected
			},
			expectOneOf: []int64{2},
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
			result := selectByModelLoad(tt.accounts, tt.preferOAuth)
			if tt.expectNil {
				require.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			require.Contains(t, tt.expectOneOf, result.account.ID)
		})
	}
}

func TestGetModelLastUsedAt(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		acc      *accountWithModelLoad
		expected time.Time
	}{
		{
			name:     "nil_model_load_info",
			acc:      &accountWithModelLoad{account: &Account{ID: 1}, modelLoadInfo: nil},
			expected: time.Time{},
		},
		{
			name:     "zero_time",
			acc:      &accountWithModelLoad{account: &Account{ID: 1}, modelLoadInfo: &ModelLoadInfo{LastUsedAt: time.Time{}}},
			expected: time.Time{},
		},
		{
			name:     "valid_time",
			acc:      &accountWithModelLoad{account: &Account{ID: 1}, modelLoadInfo: &ModelLoadInfo{LastUsedAt: now}},
			expected: now,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getModelLastUsedAt(tt.acc)
			require.Equal(t, tt.expected, result)
		})
	}
}

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

// ============ 负载分计算公式验证 ============

func TestLoadScoreFormula(t *testing.T) {
	now := time.Now()

	// 验证公式：负载分 = 有效优先级 × (1 + 模型调用次数)
	// 有效优先级 = max(1, ceil(优先级 × 0.8^未使用分钟数))
	testCases := []struct {
		priority   int
		callCount  int64
		lastUsedAt time.Time
		expected   int64
	}{
		{1, 0, now, 1},   // 1 * (1 + 0) = 1
		{1, 1, now, 2},   // 1 * (1 + 1) = 2
		{1, 10, now, 11}, // 1 * (1 + 10) = 11
		{2, 0, now, 2},   // 2 * (1 + 0) = 2
		{2, 5, now, 12},  // 2 * (1 + 5) = 12
		{3, 3, now, 12},  // 3 * (1 + 3) = 12
		{5, 10, now, 55}, // 5 * (1 + 10) = 55
	}

	for _, tc := range testCases {
		accounts := []accountWithModelLoad{
			{account: &Account{Priority: tc.priority}, modelLoadInfo: &ModelLoadInfo{CallCount: tc.callCount, LastUsedAt: tc.lastUsedAt}},
		}
		calculateLoadScores(accounts)
		require.Equal(t, tc.expected, accounts[0].loadScore,
			"priority=%d, callCount=%d", tc.priority, tc.callCount)
	}
}

// ============ 边界情况测试 ============

func TestSelectByModelLoad_LoadBalancingBehavior(t *testing.T) {
	now := time.Now()

	// 测试负载均衡行为：高优先级账号调用次数多时，低优先级账号可能被选中
	accounts := []accountWithModelLoad{
		// 高优先级(1)但调用次数多 -> score: 1*(1+20)=21
		{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 20, LastUsedAt: now}},
		// 低优先级(3)但调用次数少 -> score: 3*(1+0)=3
		{account: &Account{ID: 2, Priority: 3}, modelLoadInfo: &ModelLoadInfo{CallCount: 0, LastUsedAt: now}},
	}

	result := selectByModelLoad(accounts, false)
	require.NotNil(t, result)
	// 低优先级账号因为负载分更低，应该被选中
	require.Equal(t, int64(2), result.account.ID)
}

// TestSelectByModelLoad_PriorityDecayBehavior 测试优先级衰减行为
func TestSelectByModelLoad_PriorityDecayBehavior(t *testing.T) {
	now := time.Now()

	// 测试：低优先级账号长时间未使用后，有效优先级降低，可以和高优先级账号竞争
	accounts := []accountWithModelLoad{
		// 高优先级(1)，刚刚使用 -> 有效优先级 1 -> score: 1*(1+0)=1
		{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 0, LastUsedAt: now}},
		// 低优先级(10)，15分钟未使用 -> 有效优先级 1 -> score: 1*(1+0)=1
		{account: &Account{ID: 2, Priority: 10}, modelLoadInfo: &ModelLoadInfo{CallCount: 0, LastUsedAt: now.Add(-15 * time.Minute)}},
	}

	// 计算负载分
	calculateLoadScores(accounts)

	// 两个账号的负载分应该相同（都是 1）
	require.Equal(t, int64(1), accounts[0].loadScore)
	require.Equal(t, int64(1), accounts[1].loadScore)
}

// TestSelectByModelLoad_NeverUsedAccount 测试从未使用的账号能获得公平调度机会
func TestSelectByModelLoad_NeverUsedAccount(t *testing.T) {
	now := time.Now()

	accounts := []accountWithModelLoad{
		// 高优先级(1)，刚刚使用 -> 有效优先级 1 -> score: 1*(1+0)=1
		{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 0, LastUsedAt: now}},
		// 低优先级(100)，从未使用 -> 有效优先级 1 -> score: 1*(1+0)=1
		{account: &Account{ID: 2, Priority: 100}, modelLoadInfo: nil},
	}

	// 计算负载分
	calculateLoadScores(accounts)

	// 两个账号的负载分应该相同（都是 1）
	require.Equal(t, int64(1), accounts[0].loadScore)
	require.Equal(t, int64(1), accounts[1].loadScore)

	// 选择账号时，由于负载分相同，会按原始优先级筛选
	// 账号1（Priority=1）会被选中，因为原始优先级更高
	// 这是合理的：当负载分相同时，仍然优先选择原始优先级更高的账号
	result := selectByModelLoad(accounts, false)
	require.NotNil(t, result)
	require.Equal(t, int64(1), result.account.ID)
}

// TestSelectByModelLoad_LowPriorityGetsChance 测试低优先级账号长时间未使用后能被调度
func TestSelectByModelLoad_LowPriorityGetsChance(t *testing.T) {
	now := time.Now()

	accounts := []accountWithModelLoad{
		// 高优先级(1)，刚刚使用，有调用 -> 有效优先级 1, 调用次数 5 -> score: 1*(1+5)=6
		{account: &Account{ID: 1, Priority: 1}, modelLoadInfo: &ModelLoadInfo{CallCount: 5, LastUsedAt: now}},
		// 低优先级(10)，15分钟未使用 -> 有效优先级 1 -> score: 1*(1+0)=1
		{account: &Account{ID: 2, Priority: 10}, modelLoadInfo: &ModelLoadInfo{CallCount: 0, LastUsedAt: now.Add(-15 * time.Minute)}},
	}

	// 计算负载分
	calculateLoadScores(accounts)

	// 账号1负载分为6，账号2负载分为1
	require.Equal(t, int64(6), accounts[0].loadScore)
	require.Equal(t, int64(1), accounts[1].loadScore)

	// 账号2（低优先级但长时间未使用）应该被选中，因为负载分更低
	result := selectByModelLoad(accounts, false)
	require.NotNil(t, result)
	require.Equal(t, int64(2), result.account.ID)
}
