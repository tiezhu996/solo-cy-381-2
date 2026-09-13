package service

import (
	"testing"

	"github.com/aasplit/aasplit/internal/constants"
	"github.com/aasplit/aasplit/internal/dto"
	"github.com/aasplit/aasplit/internal/model"
	"github.com/aasplit/aasplit/internal/repository"
	"github.com/aasplit/aasplit/internal/util"
	"gorm.io/gorm"
)

// newExpenseServiceFixture 构造带群组与成员的服务夹具。
func newExpenseServiceFixture(t *testing.T) (*gorm.DB, *ExpenseService, *GroupService, uint, uint, uint, uint) {
	t.Helper()
	db := newTestDB(t)
	userRepo := repository.NewUserRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	memberRepo := repository.NewGroupMemberRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	auditSvc := NewAuditService(auditRepo, newTestLogger())
	logger := newTestLogger()

	e1 := "alice@test.com"
	e2 := "bob@test.com"
	u1 := &model.User{Username: "alice", PasswordHash: "x", Nickname: "Alice", Email: &e1, Role: constants.RoleUser}
	u2 := &model.User{Username: "bob", PasswordHash: "x", Nickname: "Bob", Email: &e2, Role: constants.RoleUser}
	u3 := &model.User{Username: "carol", PasswordHash: "x", Nickname: "Carol", Role: constants.RoleUser}
	if err := userRepo.Create(u1); err != nil {
		t.Fatalf("create u1: %v", err)
	}
	if err := userRepo.Create(u2); err != nil {
		t.Fatalf("create u2: %v", err)
	}
	if err := userRepo.Create(u3); err != nil {
		t.Fatalf("create u3: %v", err)
	}
	groupSvc := NewGroupService(db, groupRepo, memberRepo, userRepo, auditSvc, logger)
	group, err := groupSvc.Create(u1.ID, &dto.CreateGroupReq{Name: "周末聚餐", Description: "测试"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := groupSvc.InviteMember(u1.ID, group.ID, "bob"); err != nil {
		t.Fatalf("invite bob: %v", err)
	}
	if err := groupSvc.InviteMember(u1.ID, group.ID, "carol"); err != nil {
		t.Fatalf("invite carol: %v", err)
	}
	expenseSvc := NewExpenseService(db, repository.NewExpenseRepository(db), memberRepo, groupRepo, userRepo, auditSvc, logger)
	return db, expenseSvc, groupSvc, group.ID, u1.ID, u2.ID, u3.ID
}

func TestExpenseServiceCreateEqualSplit(t *testing.T) {
	_, svc, _, groupID, aliceID, bobID, carolID := newExpenseServiceFixture(t)

	tests := []struct {
		name    string
		amount  float64
		payerID uint
		shares  []dto.ShareInput
		wantSum float64
		wantErr bool
		errCode int
	}{
		{name: "three way equal 300", amount: 300, payerID: aliceID, shares: []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}, {UserID: carolID}}, wantSum: 300},
		{name: "odd 100.01", amount: 100.01, payerID: aliceID, shares: []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}, {UserID: carolID}}, wantSum: 100.01},
		{name: "non-member payer", amount: 50, payerID: 999, shares: []dto.ShareInput{{UserID: aliceID}}, wantErr: true, errCode: 40000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &dto.CreateExpenseReq{
				GroupID: groupID, Title: "火锅", Amount: tt.amount, Category: "dining",
				PayerID: tt.payerID, SplitType: "equal", PaidAt: "2026-08-01 12:00:00", Shares: tt.shares,
			}
			expense, err := svc.Create(aliceID, req)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				ae := util.AsAppError(err)
				if ae.Code != tt.errCode {
					t.Fatalf("err code = %d, want %d", ae.Code, tt.errCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			sum := 0.0
			for _, s := range expense.Shares {
				sum += s.ShareAmount
			}
			if util.Round2(sum) != tt.wantSum {
				t.Fatalf("share sum = %.2f, want %.2f", sum, tt.wantSum)
			}
		})
	}
}

func TestExpenseServiceCreateAmountMismatch(t *testing.T) {
	_, svc, _, groupID, aliceID, bobID, _ := newExpenseServiceFixture(t)
	req := &dto.CreateExpenseReq{
		GroupID: groupID, Title: "打车", Amount: 100, Category: "transport",
		PayerID: aliceID, SplitType: "amount", PaidAt: "2026-08-02 12:00:00",
		Shares: []dto.ShareInput{{UserID: aliceID, Amount: 30}, {UserID: bobID, Amount: 30}},
	}
	_, err := svc.Create(aliceID, req)
	if err == nil {
		t.Fatalf("expected mismatch error, got nil")
	}
	ae := util.AsAppError(err)
	if ae.Code != constants.CodeExpenseShareMismatch {
		t.Fatalf("err code = %d, want %d", ae.Code, constants.CodeExpenseShareMismatch)
	}
}

func TestExpenseServiceCreateShareSplit(t *testing.T) {
	_, svc, _, groupID, aliceID, bobID, carolID := newExpenseServiceFixture(t)
	req := &dto.CreateExpenseReq{
		GroupID: groupID, Title: "民宿", Amount: 100, Category: "lodging",
		PayerID: aliceID, SplitType: "share", PaidAt: "2026-08-04 12:00:00",
		Shares: []dto.ShareInput{{UserID: aliceID, Share: 1}, {UserID: bobID, Share: 1}, {UserID: carolID, Share: 1}},
	}
	expense, err := svc.Create(aliceID, req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	want := map[uint]float64{aliceID: 33.34, bobID: 33.33, carolID: 33.33}
	sum := 0.0
	for _, s := range expense.Shares {
		if s.ShareAmount != want[s.UserID] {
			t.Fatalf("user %d amount = %.2f, want %.2f", s.UserID, s.ShareAmount, want[s.UserID])
		}
		if s.ShareCount != 1 {
			t.Fatalf("user %d share count = %d, want 1", s.UserID, s.ShareCount)
		}
		sum += s.ShareAmount
	}
	if util.Round2(sum) != 100 {
		t.Fatalf("share sum = %.2f, want 100.00", sum)
	}
}

func TestExpenseServiceShareSplitInvalidShare(t *testing.T) {
	_, svc, _, groupID, aliceID, bobID, _ := newExpenseServiceFixture(t)
	req := &dto.CreateExpenseReq{
		GroupID: groupID, Title: "烧烤", Amount: 80, Category: "dining",
		PayerID: aliceID, SplitType: "share", PaidAt: "2026-08-05 12:00:00",
		Shares: []dto.ShareInput{{UserID: aliceID, Share: 1}, {UserID: bobID, Share: 0}},
	}
	_, err := svc.Create(aliceID, req)
	if err == nil {
		t.Fatalf("expected invalid split error, got nil")
	}
	ae := util.AsAppError(err)
	if ae.Code != constants.CodeExpenseInvalidSplit {
		t.Fatalf("err code = %d, want %d", ae.Code, constants.CodeExpenseInvalidSplit)
	}
}

func TestExpenseServiceUpdateToShareSplit(t *testing.T) {
	_, svc, _, groupID, aliceID, bobID, carolID := newExpenseServiceFixture(t)
	createReq := &dto.CreateExpenseReq{
		GroupID: groupID, Title: "门票", Amount: 90, Category: "entertain",
		PayerID: aliceID, SplitType: "equal", PaidAt: "2026-08-06 12:00:00",
		Shares: []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}, {UserID: carolID}},
	}
	expense, err := svc.Create(aliceID, createReq)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updateReq := &dto.UpdateExpenseReq{
		Title: "门票", Amount: 90, Category: "entertain",
		PayerID: aliceID, SplitType: "share", PaidAt: "2026-08-06 12:00:00",
		Shares: []dto.ShareInput{{UserID: aliceID, Share: 2}, {UserID: bobID, Share: 1}, {UserID: carolID, Share: 1}},
	}
	if err := svc.Update(aliceID, expense.ID, updateReq); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := svc.Get(aliceID, expense.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SplitType != constants.SplitShare {
		t.Fatalf("split type = %s, want share", got.SplitType)
	}
	want := map[uint]float64{aliceID: 45, bobID: 22.5, carolID: 22.5}
	wantShare := map[uint]int{aliceID: 2, bobID: 1, carolID: 1}
	sum := 0.0
	for _, s := range got.Shares {
		if s.ShareAmount != want[s.UserID] {
			t.Fatalf("user %d amount = %.2f, want %.2f", s.UserID, s.ShareAmount, want[s.UserID])
		}
		if s.ShareCount != wantShare[s.UserID] {
			t.Fatalf("user %d share count = %d, want %d", s.UserID, s.ShareCount, wantShare[s.UserID])
		}
		sum += s.ShareAmount
	}
	if util.Round2(sum) != 90 {
		t.Fatalf("share sum = %.2f, want 90.00", sum)
	}
}

func TestExpenseServiceDeleteAndStatus(t *testing.T) {
	_, svc, _, groupID, aliceID, bobID, _ := newExpenseServiceFixture(t)
	req := &dto.CreateExpenseReq{
		GroupID: groupID, Title: "电影", Amount: 80, Category: "entertain",
		PayerID: aliceID, SplitType: "equal", PaidAt: "2026-08-03 12:00:00",
		Shares: []dto.ShareInput{{UserID: aliceID}, {UserID: bobID}},
	}
	expense, err := svc.Create(aliceID, req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Delete(aliceID, expense.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, err := svc.Get(aliceID, expense.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != constants.ExpenseRefunded {
		t.Fatalf("status = %s, want refunded", got.Status)
	}
}
