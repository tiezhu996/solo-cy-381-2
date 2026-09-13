package model

import (
	"time"
)

// ShareStatus 分摊明细状态。
type ShareStatus string

const (
	ShareUnsettled ShareStatus = "unsettled" // 未结算
	ShareSettled   ShareStatus = "settled"   // 已结算
)

// ExpenseShare 消费分摊明细实体：每人应付金额与比例。
type ExpenseShare struct {
	ID          uint        `gorm:"primaryKey" json:"id"`
	ExpenseID   uint        `gorm:"index:idx_share_expense_user,unique;not null" json:"expense_id"`
	UserID      uint        `gorm:"index:idx_share_expense_user,unique;not null" json:"user_id"`
	ShareAmount float64     `gorm:"type:double precision;not null" json:"share_amount"`
	Ratio       float64     `gorm:"type:double precision;default:0" json:"ratio"`
	ShareCount  int         `gorm:"default:0" json:"share_count"` // 按份额分摊时参与人填写的份额
	Status      ShareStatus `gorm:"size:16;not null;default:unsettled" json:"status"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName 指定表名。
func (ExpenseShare) TableName() string { return "expense_shares" }
