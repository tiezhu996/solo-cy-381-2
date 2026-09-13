package service

import (
	"testing"

	"github.com/aasplit/aasplit/internal/dto"
	"github.com/aasplit/aasplit/internal/repository"
)

func TestSettlementServiceGenerate(t *testing.T) {
	db, expenseSvc, _, groupID, aliceID, bobID, carolID := newExpenseServiceFixture(t)

	// Alice 付 300 三人均摊 → Bob 应付 100，Carol 应付 100
	req := &dto.CreateExpenseReq{
		GroupID: groupID, Title: "火锅", Amount: 300, Category: "dining",
		PayerID: aliceID, SplitType: "equal", PaidAt: "2026-08-01 12:00:00",
		Shares: []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}, {UserID: carolID}},
	}
	if _, err := expenseSvc.Create(aliceID, req); err != nil {
		t.Fatalf("create expense: %v", err)
	}

	settleRepo := repository.NewSettlementRepository(db)
	shareRepo := repository.NewExpenseShareRepository(db)
	memberRepo := repository.NewGroupMemberRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	userRepo := repository.NewUserRepository(db)
	auditSvc := NewAuditService(repository.NewAuditRepository(db), newTestLogger())
	svc := NewSettlementService(db, settleRepo, shareRepo, memberRepo, groupRepo, userRepo, auditSvc, newTestLogger())

	items, err := svc.Generate(aliceID, groupID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	total := 0.0
	for _, it := range items {
		total += it.Amount
		if it.Status != "pending" {
			t.Fatalf("status = %s, want pending", it.Status)
		}
	}
	if len(items) != 2 {
		t.Fatalf("transfers = %d, want 2", len(items))
	}
	if total < 199.99 || total > 200.01 {
		t.Fatalf("transfer total = %.2f, want ~200", total)
	}

	affected, err := svc.Settle(aliceID, &dto.SettleReq{SettlementIDs: []uint{items[0].ID, items[1].ID}})
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if affected != 2 {
		t.Fatalf("affected = %d, want 2", affected)
	}
	pending, err := svc.ListPending(bobID)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending after settle = %d, want 0", len(pending))
	}
}

// TestSettlementServiceGenerateWithShareSplit 结算建议必须按份额分摊结果计算净余额。
func TestSettlementServiceGenerateWithShareSplit(t *testing.T) {
	tests := []struct {
		name   string
		amount float64
		shares []int // 与 alice/bob/carol 顺序对应的份额
	}{
		{
			name:   "份额2:1:1 总额90",
			amount: 90,
			shares: []int{2, 1, 1},
			// alice 付90 应付45 → 净+45；bob/carol 各应付22.5 → 各转给 alice 22.5
		},
		{
			name:   "份额1:1:1 总额100（尾差给付款人）",
			amount: 100,
			shares: []int{1, 1, 1},
			// alice 付100 应付33.34 → 净+66.66；bob/carol 各应付33.33
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, expenseSvc, _, groupID, aliceID, bobID, carolID := newExpenseServiceFixture(t)
			ids := []uint{aliceID, bobID, carolID}
			inputs := make([]dto.ShareInput, 0, 3)
			for i, id := range ids {
				inputs = append(inputs, dto.ShareInput{UserID: id, Share: tt.shares[i]})
			}
			req := &dto.CreateExpenseReq{
				GroupID: groupID, Title: "按份额消费", Amount: tt.amount, Category: "dining",
				PayerID: aliceID, SplitType: "share", PaidAt: "2026-08-09 12:00:00", Shares: inputs,
			}
			if _, err := expenseSvc.Create(aliceID, req); err != nil {
				t.Fatalf("create expense: %v", err)
			}
			svc := NewSettlementService(db, repository.NewSettlementRepository(db), repository.NewExpenseShareRepository(db),
				repository.NewGroupMemberRepository(db), repository.NewGroupRepository(db), repository.NewUserRepository(db),
				NewAuditService(repository.NewAuditRepository(db), newTestLogger()), newTestLogger())

			// 先校验净余额：付款人净额 = 总额 - 自己应付。
			balances, err := svc.Balances(aliceID, groupID)
			if err != nil {
				t.Fatalf("balances: %v", err)
			}
			net := make(map[uint]float64, len(balances))
			for _, b := range balances {
				net[b.UserID] = b.NetAmount
			}
			// 由份额结果推期望净额：应付按 splitcalc 规则计算。
			expenses, _, err := expenseSvc.List(aliceID, groupID, &dto.ExpenseQuery{Page: 1, PageSize: 10})
			if err != nil {
				t.Fatalf("list expense: %v", err)
			}
			if len(expenses) != 1 {
				t.Fatalf("expenses = %d, want 1", len(expenses))
			}
			owed := make(map[uint]float64, 3)
			for _, s := range expenses[0].Shares {
				owed[s.UserID] = s.ShareAmount
			}
			for _, id := range ids {
				want := 0.0 - owed[id]
				if id == aliceID {
					want = tt.amount - owed[id]
				}
				if net[id] != want {
					t.Errorf("违反业务规则[结算按份额分摊结果计算净余额]: user %d 净余额 %.2f, want %.2f", id, net[id], want)
				}
			}

			// 再校验生成的转账：债务人 → 付款人，金额等于其应付。
			items, err := svc.Generate(aliceID, groupID)
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			wantTransfers := map[[2]uint]float64{
				{bobID, aliceID}:   owed[bobID],
				{carolID, aliceID}: owed[carolID],
			}
			if len(items) != len(wantTransfers) {
				t.Fatalf("违反业务规则[结算按份额分摊结果生成转账]: 转账数 = %d, want %d", len(items), len(wantTransfers))
			}
			for _, it := range items {
				key := [2]uint{it.FromUserID, it.ToUserID}
				want, ok := wantTransfers[key]
				if !ok {
					t.Errorf("违反业务规则[结算按份额分摊结果生成转账]: 意外转账 %d→%d %.2f", it.FromUserID, it.ToUserID, it.Amount)
					continue
				}
				if it.Amount != want {
					t.Errorf("违反业务规则[结算按份额分摊结果生成转账]: 转账 %d→%d 金额 %.2f, want %.2f", it.FromUserID, it.ToUserID, it.Amount, want)
				}
				delete(wantTransfers, key)
			}
			for key, amt := range wantTransfers {
				t.Errorf("违反业务规则[结算按份额分摊结果生成转账]: 缺少转账 %d→%d %.2f", key[0], key[1], amt)
			}
		})
	}
}
