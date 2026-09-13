package splitcalc

import (
	"math"
	"testing"
)

// 按份额分摊业务规则（断言消息直接引用规则编号，失败时可定位被破坏的规则）：
// R1 每个参与人的份额必须为正整数，否则拒绝；
// R2 应付金额按份额占比计算；
// R3 尾差（分）依次分给份额更高的参与人，份额相同时按参与人名单顺序分配；
// R4 所有人应付金额合计必须等于消费总额；
// R5 份额或总额超出可靠计算范围时必须拒绝，不得产生负数或错误占比。
const (
	rulePositiveShare = "R1 份额必须为正整数"
	ruleProportional  = "R2 应付金额按份额占比计算"
	ruleRemainder     = "R3 尾差依次分给份额更高者，同份额按名单顺序"
	ruleSumTotal      = "R4 所有人应付金额合计必须等于消费总额"
	ruleOverflow      = "R5 超出可靠范围必须拒绝，不得产生负数或错误占比"
)

// requireShareResults 校验按份额结果：金额符合期望、份额回写正确、合计等于总额（R4）。
func requireShareResults(t *testing.T, rule string, total float64, parts []Participant, want []float64, shares []Share) {
	t.Helper()
	if len(shares) != len(parts) {
		t.Fatalf("违反业务规则[%s]: 结果人数 = %d, 参与人 = %d", rule, len(shares), len(parts))
	}
	sum := 0.0
	for i, s := range shares {
		if s.UserID != parts[i].UserID {
			t.Fatalf("违反业务规则[%s]: 结果顺序应与名单一致, shares[%d].UserID = %d, want %d", ruleRemainder, i, s.UserID, parts[i].UserID)
		}
		if s.ShareAmount != want[i] {
			t.Errorf("违反业务规则[%s]: user_id=%d 应付 %.2f, 应为 %.2f", rule, s.UserID, s.ShareAmount, want[i])
		}
		if s.ShareAmount < 0 {
			t.Errorf("违反业务规则[%s]: user_id=%d 应付金额为负数 %.2f", ruleOverflow, s.UserID, s.ShareAmount)
		}
		if s.Ratio < 0 || s.Ratio > 1 {
			t.Errorf("违反业务规则[%s]: user_id=%d 占比 %.4f 超出 [0,1]", ruleOverflow, s.UserID, s.Ratio)
		}
		if s.ShareCount != parts[i].Share {
			t.Errorf("违反业务规则[%s]: user_id=%d 回写份额 = %d, 填写份额 = %d", ruleProportional, s.UserID, s.ShareCount, parts[i].Share)
		}
		sum += s.ShareAmount
	}
	if round2(sum) != round2(total) {
		t.Errorf("违反业务规则[%s]: 应付合计 %.2f, 消费总额 %.2f", ruleSumTotal, sum, total)
	}
}

// TestShareSplitNormal 正常份额：按占比计算（R2），合计等于总额（R4）。
func TestShareSplitNormal(t *testing.T) {
	tests := []struct {
		name  string
		total float64
		parts []Participant
		want  []float64
	}{
		{"1:2:1 总额100", 100, []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: 2}, {UserID: 3, Share: 1}}, []float64{25, 50, 25}},
		{"2:1:1 总额90", 90, []Participant{{UserID: 1, Share: 2}, {UserID: 2, Share: 1}, {UserID: 3, Share: 1}}, []float64{45, 22.5, 22.5}},
		{"单人份额5 总额80", 80, []Participant{{UserID: 1, Share: 5}}, []float64{80}},
		{"3:1:1 总额50", 50, []Participant{{UserID: 1, Share: 3}, {UserID: 2, Share: 1}, {UserID: 3, Share: 1}}, []float64{30, 10, 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shares, err := CalculateShares(tt.total, SplitShare, tt.parts)
			if err != nil {
				t.Fatalf("正常份额不应报错: %v", err)
			}
			requireShareResults(t, ruleProportional, tt.total, tt.parts, tt.want, shares)
		})
	}
}

// TestShareSplitRemainder 尾差分配：分给份额更高者，同份额按名单顺序（R3），合计等于总额（R4）。
func TestShareSplitRemainder(t *testing.T) {
	tests := []struct {
		name  string
		total float64
		parts []Participant
		want  []float64
	}{
		{"同份额尾差给名单首位", 100, []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: 1}, {UserID: 3, Share: 1}}, []float64{33.34, 33.33, 33.33}},
		{"尾差给份额更高者而非名单首位", 7.11, []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: 2}, {UserID: 3, Share: 2}}, []float64{1.42, 2.85, 2.84}},
		{"同高份额按名单顺序", 7.11, []Participant{{UserID: 1, Share: 2}, {UserID: 2, Share: 2}, {UserID: 3, Share: 1}}, []float64{2.85, 2.84, 1.42}},
		{"负尾差由名单末位同份额者吸收", 33.33, []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: 1}}, []float64{16.67, 16.66}},
		{"极小金额负尾差不扣成负数", 0.02, []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: 1}, {UserID: 3, Share: 1}}, []float64{0.01, 0.01, 0}},
		{"奇数分100.01三人", 100.01, []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: 1}, {UserID: 3, Share: 1}}, []float64{33.34, 33.34, 33.33}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shares, err := CalculateShares(tt.total, SplitShare, tt.parts)
			if err != nil {
				t.Fatalf("正常份额不应报错: %v", err)
			}
			requireShareResults(t, ruleRemainder, tt.total, tt.parts, tt.want, shares)
		})
	}
}

// TestShareSplitRejectsNonPositive 非正整数份额必须拒绝（R1）。
func TestShareSplitRejectsNonPositive(t *testing.T) {
	tests := []struct {
		name  string
		parts []Participant
	}{
		{"份额为0", []Participant{{UserID: 1, Share: 0}}},
		{"份额为负", []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: -2}}},
		{"部分份额为0", []Participant{{UserID: 1, Share: 2}, {UserID: 2, Share: 0}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CalculateShares(100, SplitShare, tt.parts); err != ErrInvalidSplit {
				t.Errorf("违反业务规则[%s]: err = %v, 应拒绝(ErrInvalidSplit)", rulePositiveShare, err)
			}
		})
	}
}

// TestShareSplitRejectsOverflow 极大份额/总额超出可靠范围必须拒绝（R5）。
func TestShareSplitRejectsOverflow(t *testing.T) {
	tests := []struct {
		name  string
		total float64
		parts []Participant
	}{
		{"单个MaxInt64份额", 100, []Participant{{UserID: 1, Share: math.MaxInt64}}},
		{"份额乘总额分溢出", 100, []Participant{{UserID: 1, Share: 1 << 60}, {UserID: 2, Share: 1}}},
		{"份额总和溢出", 0.01, []Participant{{UserID: 1, Share: math.MaxInt64}, {UserID: 2, Share: math.MaxInt64}}},
		{"总额换分溢出", 1e17, []Participant{{UserID: 1, Share: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CalculateShares(tt.total, SplitShare, tt.parts); err != ErrInvalidSplit {
				t.Errorf("违反业务规则[%s]: err = %v, 应拒绝(ErrInvalidSplit)", ruleOverflow, err)
			}
		})
	}
}

// TestShareSplitLargeButValid 可靠范围内的大份额：允许且结果非负、占比合法、合计精确（R4/R5）。
func TestShareSplitLargeButValid(t *testing.T) {
	// 总额 0.01、单个极大份额：1 分 × MaxInt64 不溢出。
	parts := []Participant{{UserID: 1, Share: math.MaxInt64}}
	shares, err := CalculateShares(0.01, SplitShare, parts)
	if err != nil {
		t.Fatalf("可靠范围内的大份额不应拒绝: %v", err)
	}
	requireShareResults(t, ruleOverflow, 0.01, parts, []float64{0.01}, shares)

	// 1e14 : 1，总额 100：高份额者承担全部，低份额者为 0。
	parts = []Participant{{UserID: 1, Share: 100_000_000_000_000}, {UserID: 2, Share: 1}}
	shares, err = CalculateShares(100, SplitShare, parts)
	if err != nil {
		t.Fatalf("可靠范围内的大份额不应拒绝: %v", err)
	}
	requireShareResults(t, ruleOverflow, 100, parts, []float64{100, 0}, shares)
}
