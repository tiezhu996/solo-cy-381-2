package splitcalc

import (
	"math"
	"testing"
)

func TestCalculateSharesEqual(t *testing.T) {
	tests := []struct {
		name     string
		total    float64
		users    []uint
		wantSum  float64
		wantEach float64
	}{
		{"three way 90", 90, []uint{1, 2, 3}, 90, 30},
		{"two way 100", 100, []uint{1, 2}, 100, 50},
		{"odd cents 100.01 three", 100.01, []uint{1, 2, 3}, 100.01, 33.34},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := make([]Participant, 0, len(tt.users))
			for _, u := range tt.users {
				ps = append(ps, Participant{UserID: u})
			}
			shares, err := CalculateShares(tt.total, SplitEqual, ps)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			sum := 0.0
			for _, s := range shares {
				sum += s.ShareAmount
			}
			if round2(sum) != tt.wantSum {
				t.Fatalf("sum = %.2f, want %.2f", sum, tt.wantSum)
			}
		})
	}
}

func TestCalculateSharesRatio(t *testing.T) {
	shares, err := CalculateShares(100, SplitRatio, []Participant{
		{UserID: 1, Ratio: 1},
		{UserID: 2, Ratio: 2},
		{UserID: 3, Ratio: 1},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[uint]float64{1: 25, 2: 50, 3: 25}
	for _, s := range shares {
		if s.ShareAmount != want[s.UserID] {
			t.Fatalf("user %d amount = %.2f, want %.2f", s.UserID, s.ShareAmount, want[s.UserID])
		}
	}
}

func TestCalculateSharesAmountMismatch(t *testing.T) {
	_, err := CalculateShares(100, SplitAmount, []Participant{
		{UserID: 1, Amount: 30},
		{UserID: 2, Amount: 30},
	})
	if err != ErrSumMismatch {
		t.Fatalf("err = %v, want ErrSumMismatch", err)
	}
}

func TestCalculateSharesByShare(t *testing.T) {
	tests := []struct {
		name  string
		total float64
		parts []Participant
		want  []float64 // 按名单顺序的应付金额
	}{
		{
			name:  "proportional 1-2-1",
			total: 100,
			parts: []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: 2}, {UserID: 3, Share: 1}},
			want:  []float64{25, 50, 25},
		},
		{
			name:  "equal shares remainder to first in list",
			total: 100,
			parts: []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: 1}, {UserID: 3, Share: 1}},
			want:  []float64{33.34, 33.33, 33.33},
		},
		{
			name:  "remainder to higher share not list head",
			total: 7.11,
			parts: []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: 2}, {UserID: 3, Share: 2}},
			want:  []float64{1.42, 2.85, 2.84},
		},
		{
			name:  "tied higher shares remainder by list order",
			total: 7.11,
			parts: []Participant{{UserID: 1, Share: 2}, {UserID: 2, Share: 2}, {UserID: 3, Share: 1}},
			want:  []float64{2.85, 2.84, 1.42},
		},
		{
			name:  "negative remainder absorbed by last equal share",
			total: 33.33,
			parts: []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: 1}},
			want:  []float64{16.67, 16.66},
		},
		{
			name:  "tiny amount negative remainder",
			total: 0.02,
			parts: []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: 1}, {UserID: 3, Share: 1}},
			want:  []float64{0.01, 0.01, 0},
		},
		{
			name:  "odd cents 100.01 three way",
			total: 100.01,
			parts: []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: 1}, {UserID: 3, Share: 1}},
			want:  []float64{33.34, 33.34, 33.33},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shares, err := CalculateShares(tt.total, SplitShare, tt.parts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(shares) != len(tt.want) {
				t.Fatalf("got %d shares, want %d", len(shares), len(tt.want))
			}
			sum := 0.0
			for i, s := range shares {
				if s.UserID != tt.parts[i].UserID {
					t.Fatalf("shares[%d].UserID = %d, want %d（结果应保持名单顺序）", i, s.UserID, tt.parts[i].UserID)
				}
				if s.ShareAmount != tt.want[i] {
					t.Fatalf("user %d amount = %.2f, want %.2f", s.UserID, s.ShareAmount, tt.want[i])
				}
				if s.ShareCount != tt.parts[i].Share {
					t.Fatalf("user %d share count = %d, want %d", s.UserID, s.ShareCount, tt.parts[i].Share)
				}
				sum += s.ShareAmount
			}
			if round2(sum) != round2(tt.total) {
				t.Fatalf("sum = %.2f, want %.2f（合计必须等于消费总额）", sum, tt.total)
			}
		})
	}
}

func TestCalculateSharesByShareInvalid(t *testing.T) {
	if _, err := CalculateShares(100, SplitShare, []Participant{{UserID: 1, Share: 0}}); err != ErrInvalidSplit {
		t.Fatalf("err = %v, want ErrInvalidSplit", err)
	}
	if _, err := CalculateShares(100, SplitShare, []Participant{{UserID: 1, Share: 1}, {UserID: 2, Share: -2}}); err != ErrInvalidSplit {
		t.Fatalf("err = %v, want ErrInvalidSplit", err)
	}
}

func TestCalculateSharesByShareOverflow(t *testing.T) {
	tests := []struct {
		name  string
		total float64
		parts []Participant
	}{
		{"single max int64 share", 100, []Participant{{UserID: 1, Share: math.MaxInt64}}},
		{"share times cents overflows", 100, []Participant{{UserID: 1, Share: 1 << 60}, {UserID: 2, Share: 1}}},
		{"share sum overflows", 0.01, []Participant{{UserID: 1, Share: math.MaxInt64}, {UserID: 2, Share: math.MaxInt64}}},
		{"huge total cents overflows", 1e17, []Participant{{UserID: 1, Share: 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CalculateShares(tt.total, SplitShare, tt.parts); err != ErrInvalidSplit {
				t.Fatalf("err = %v, want ErrInvalidSplit（超出可靠范围应拒绝）", err)
			}
		})
	}
}

func TestCalculateSharesByShareLargeButValid(t *testing.T) {
	// 总额 0.01、单个极大份额：1 分 × MaxInt64 不溢出，允许且结果非负。
	shares, err := CalculateShares(0.01, SplitShare, []Participant{{UserID: 1, Share: math.MaxInt64}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shares[0].ShareAmount != 0.01 || shares[0].Ratio != 1 {
		t.Fatalf("got %+v, want amount 0.01 ratio 1", shares[0])
	}
	// 大份额但可可靠计算：1e14 : 1，总额 100。
	shares, err = CalculateShares(100, SplitShare, []Participant{{UserID: 1, Share: 100_000_000_000_000}, {UserID: 2, Share: 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sum := 0.0
	for _, s := range shares {
		if s.ShareAmount < 0 || s.Ratio < 0 || s.Ratio > 1 {
			t.Fatalf("negative or invalid result: %+v", s)
		}
		sum += s.ShareAmount
	}
	if shares[0].ShareAmount != 100 || shares[1].ShareAmount != 0 {
		t.Fatalf("got %.2f / %.2f, want 100.00 / 0.00", shares[0].ShareAmount, shares[1].ShareAmount)
	}
	if round2(sum) != 100 {
		t.Fatalf("sum = %.2f, want 100.00", sum)
	}
}

func TestCalculateSharesInvalid(t *testing.T) {
	if _, err := CalculateShares(0, SplitEqual, []Participant{{UserID: 1}}); err != ErrInvalidSplit {
		t.Fatalf("err = %v, want ErrInvalidSplit", err)
	}
	if _, err := CalculateShares(100, SplitRatio, []Participant{{UserID: 1, Ratio: 0}}); err != ErrInvalidSplit {
		t.Fatalf("err = %v, want ErrInvalidSplit", err)
	}
}

func TestOptimizeTransfers(t *testing.T) {
	balances := []Balance{
		{UserID: 1, Amount: 100},
		{UserID: 2, Amount: -30},
		{UserID: 3, Amount: -70},
	}
	transfers := OptimizeTransfers(balances)
	sum := 0.0
	for _, tr := range transfers {
		sum += tr.Amount
		if tr.FromUserID == 0 || tr.ToUserID == 0 {
			t.Fatalf("transfer has zero user: %+v", tr)
		}
	}
	if round2(sum) != 100 {
		t.Fatalf("transfer sum = %.2f, want 100", sum)
	}
	if len(transfers) > 2 {
		t.Fatalf("transfers = %d, want <= 2", len(transfers))
	}
}

func TestOptimizeTransfersEmpty(t *testing.T) {
	if got := OptimizeTransfers(nil); len(got) != 0 {
		t.Fatalf("expected no transfers, got %d", len(got))
	}
}
