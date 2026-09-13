// Package splitcalc 提供 AA 分摊计算与智能结算路径优化算法（无业务依赖，可复用）。
package splitcalc

import (
	"errors"
	"math"
	"sort"
)

// SplitType 分摊方式。
type SplitType string

const (
	SplitEqual  SplitType = "equal"  // 均摊
	SplitRatio  SplitType = "ratio"  // 按比例
	SplitAmount SplitType = "amount" // 按金额
	SplitShare  SplitType = "share"  // 按份额
)

// Participant 分摊参与人。
type Participant struct {
	UserID uint    `json:"user_id"`
	Ratio  float64 `json:"ratio,omitempty"`  // ratio 模式下占比
	Amount float64 `json:"amount,omitempty"` // amount 模式下应付金额
	Share  int     `json:"share,omitempty"`  // share 模式下份额（正整数）
}

// Share 单人的分摊结果。
type Share struct {
	UserID      uint    `json:"user_id"`
	ShareAmount float64 `json:"share_amount"`
	Ratio       float64 `json:"ratio"`
	ShareCount  int     `json:"share_count"` // share 模式下参与人填写的份额
}

// ErrInvalidSplit 分摊参数无效。
var ErrInvalidSplit = errors.New("invalid split parameters")

// ErrSumMismatch 分摊金额合计与消费总额不匹配。
var ErrSumMismatch = errors.New("share sum mismatch")

// round2 四舍五入保留两位小数。
func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// CalculateShares 根据分摊方式计算每人应付金额；保证合计等于总额（最后一人吸收舍入误差）。
func CalculateShares(total float64, splitType SplitType, participants []Participant) ([]Share, error) {
	if total <= 0 {
		return nil, ErrInvalidSplit
	}
	if len(participants) == 0 {
		return nil, ErrInvalidSplit
	}
	shares := make([]Share, 0, len(participants))
	switch splitType {
	case SplitEqual:
		base := round2(total / float64(len(participants)))
		allocated := 0.0
		for i, p := range participants {
			amount := base
			if i == len(participants)-1 {
				amount = round2(total - allocated)
			}
			shares = append(shares, Share{UserID: p.UserID, ShareAmount: amount, Ratio: round2(1 / float64(len(participants)))})
			allocated = round2(allocated + amount)
		}
	case SplitRatio:
		sumRatio := 0.0
		for _, p := range participants {
			if p.Ratio < 0 {
				return nil, ErrInvalidSplit
			}
			sumRatio += p.Ratio
		}
		if sumRatio <= 0 {
			return nil, ErrInvalidSplit
		}
		allocated := 0.0
		for i, p := range participants {
			amount := round2(total * p.Ratio / sumRatio)
			if i == len(participants)-1 {
				amount = round2(total - allocated)
			}
			shares = append(shares, Share{UserID: p.UserID, ShareAmount: amount, Ratio: round2(p.Ratio / sumRatio)})
			allocated = round2(allocated + amount)
		}
	case SplitAmount:
		sumAmount := 0.0
		for _, p := range participants {
			if p.Amount < 0 {
				return nil, ErrInvalidSplit
			}
			sumAmount += p.Amount
		}
		if round2(sumAmount) != round2(total) {
			return nil, ErrSumMismatch
		}
		for _, p := range participants {
			ratio := 0.0
			if total > 0 {
				ratio = round2(p.Amount / total)
			}
			shares = append(shares, Share{UserID: p.UserID, ShareAmount: round2(p.Amount), Ratio: ratio})
		}
	case SplitShare:
		return calculateByShare(total, participants)
	default:
		return nil, ErrInvalidSplit
	}
	return shares, nil
}

// calculateByShare 按份额分摊：应付金额按份额占比计算，尾差（分）依次分给份额更高的
// 参与人，份额相同时按参与人名单顺序分配，保证合计严格等于消费总额。
// 计算以分为单位做 int64 整数运算；份额或总额过大导致 总额分×总份额 可能溢出
// int64 时视为超出可靠范围，直接返回 ErrInvalidSplit 拒绝。
func calculateByShare(total float64, participants []Participant) ([]Share, error) {
	// 总额换算成分后必须落在 int64 可表示范围内。
	centsF := total*100 + 0.5
	if centsF >= math.MaxInt64 {
		return nil, ErrInvalidSplit
	}
	totalCents := int64(centsF)
	// 校验份额为正整数，且份额总和、总额分×总份额 均不溢出 int64。
	sumShare := int64(0)
	for _, p := range participants {
		if p.Share <= 0 {
			return nil, ErrInvalidSplit
		}
		if int64(p.Share) > math.MaxInt64-sumShare {
			return nil, ErrInvalidSplit
		}
		sumShare += int64(p.Share)
	}
	if totalCents > 0 && sumShare > math.MaxInt64/totalCents {
		return nil, ErrInvalidSplit
	}
	cents := make([]int64, len(participants))
	allocated := int64(0)
	for i, p := range participants {
		// 四舍五入：q = totalCents*share/sumShare，余数两倍不小于除数则进一。
		prod := totalCents * int64(p.Share)
		q := prod / sumShare
		if r := prod % sumShare; r >= sumShare-r {
			q++
		}
		cents[i] = q
		allocated += q
	}
	// 分配顺序：份额降序，份额相同保持名单先后顺序。
	order := make([]int, len(participants))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return participants[order[i]].Share > participants[order[j]].Share })
	remainder := totalCents - allocated
	for remainder > 0 {
		for _, idx := range order {
			if remainder <= 0 {
				break
			}
			cents[idx]++
			remainder--
		}
	}
	for remainder < 0 {
		// 尾差为负时从份额低者开始扣减（不扣成负数）。
		progressed := false
		for k := len(order) - 1; k >= 0 && remainder < 0; k-- {
			idx := order[k]
			if cents[idx] > 0 {
				cents[idx]--
				remainder++
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	shares := make([]Share, 0, len(participants))
	for i, p := range participants {
		shares = append(shares, Share{
			UserID:      p.UserID,
			ShareAmount: float64(cents[i]) / 100,
			Ratio:       round2(float64(p.Share) / float64(sumShare)),
			ShareCount:  p.Share,
		})
	}
	return shares, nil
}

// Balance 成员净余额（正数应收，负数应付）。
type Balance struct {
	UserID uint
	Amount float64
}

// Transfer 一条结算转账。
type Transfer struct {
	FromUserID uint
	ToUserID   uint
	Amount     float64
}

// OptimizeTransfers 基于净余额贪心匹配最大债权人与最大债务人，最小化转账次数。
func OptimizeTransfers(balances []Balance) []Transfer {
	creditors := make([]Balance, 0, len(balances))
	debtors := make([]Balance, 0, len(balances))
	for _, b := range balances {
		if round2(b.Amount) > 0 {
			creditors = append(creditors, b)
		} else if round2(b.Amount) < 0 {
			debtors = append(debtors, Balance{UserID: b.UserID, Amount: -b.Amount})
		}
	}
	sort.Slice(creditors, func(i, j int) bool { return creditors[i].Amount > creditors[j].Amount })
	sort.Slice(debtors, func(i, j int) bool { return debtors[i].Amount > debtors[j].Amount })

	transfers := make([]Transfer, 0, len(balances))
	i, j := 0, 0
	for i < len(debtors) && j < len(creditors) {
		pay := round2(debtors[i].Amount)
		recv := round2(creditors[j].Amount)
		amount := pay
		if recv < pay {
			amount = recv
		}
		amount = round2(amount)
		if amount > 0 {
			transfers = append(transfers, Transfer{FromUserID: debtors[i].UserID, ToUserID: creditors[j].UserID, Amount: amount})
		}
		debtors[i].Amount = round2(debtors[i].Amount - amount)
		creditors[j].Amount = round2(creditors[j].Amount - amount)
		if round2(debtors[i].Amount) <= 0 {
			i++
		}
		if round2(creditors[j].Amount) <= 0 {
			j++
		}
	}
	return transfers
}
