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
	MccByBankCategory(ctx context.Context, source domain.Source, bankCategory string) (*domain.BankCategoryMCC, error)
	// TrackUnmapped запам'ятовує ключ, для якого не знайшлося мапінгу,
	// щоб було видно, яке правило варто дописати. Повторний ключ лише збільшує лічильник.
	TrackUnmapped(ctx context.Context, key domain.UnmappedKey) error
	ListUnmapped(ctx context.Context, filter domain.UnmappedFilter) ([]domain.UnmappedKey, error)
	AddRule(ctx context.Context, rule domain.MCCCategoryRule) (*domain.MCCCategoryRule, error)
}

// CategoryCatalog — довідник категорій ezBookkeeping. Потрібен лише UI мапінгу:
// щоб людина обирала категорію зі списку назв, а не вписувала id руками.
type CategoryCatalog interface {
	Categories(ctx context.Context) ([]domain.EzCategory, error)
}

type Bookkeeper interface {
	AddTransaction(ctx context.Context, tx *domain.BankTransaction, categoryID, accountID string) error
}
