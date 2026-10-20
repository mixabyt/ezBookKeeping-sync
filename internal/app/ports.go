package app

import (
	"context"

	"ezbkTools/internal/domain"
)

type JobRepository interface {
	Add(ctx context.Context, tx *domain.BankTransaction) error
	FetchPending(ctx context.Context, limit int) ([]domain.Job, error)
	MarkDone(ctx context.Context, id int64) error
	MarkFailed(ctx context.Context, id int64, err error) error
}

// CategoryRepository — мапінг MCC → категорія ezBookkeeping (зберігається в БД).
type CategoryRepository interface {
	EzCategoryRuleByMCC(ctx context.Context, mcc int) (*domain.MCCCategoryRule, error)
	MccByBankCategory(source domain.Source, bankCategory string) (*domain.BankCategoryMCC, error)
}

type Bookkeeper interface {
	AddTransaction(ctx context.Context, tx *domain.BankTransaction, categoryID string) error
}
