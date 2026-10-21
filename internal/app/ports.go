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

	// MarkWaiting — для джоби бракує мапінгу; reason пояснює, чого саме.
	MarkWaiting(ctx context.Context, id int64, reason string) error
	// FetchWaiting — з курсором afterID: набір waiting-джоб не зменшується від обробки.
	FetchWaiting(ctx context.Context, afterID int64, limit int) ([]domain.Job, error)
	// WaitingCounts / WaitingAccounts — що саме блокує джоби, для лічильників у UI.
	WaitingCounts(ctx context.Context) (map[domain.WaitingKey]int, error)
	WaitingAccounts(ctx context.Context) ([]domain.WaitingAccount, error)
}

// AccountRepository — мапінг банківського рахунку на рахунок ezBookkeeping.
// Дефолту немає навмисно: запис не на той рахунок ламає баланс.
type AccountRepository interface {
	EzAccountByBank(ctx context.Context, source domain.Source, bankAccountID string) (*domain.AccountMapping, error)
	AddMapping(ctx context.Context, mapping domain.AccountMapping) error
	ListMappings(ctx context.Context) ([]domain.AccountMapping, error)
}

// JobWaker — розбудити воркер, щоб він одразу перевірив джоби, а не чекав тіку.
// Окремий порт, щоб MappingService не залежав від SyncService.
type JobWaker interface {
	Wake()
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
	Accounts(ctx context.Context) ([]domain.EzAccount, error)
}

type Bookkeeper interface {
	AddTransaction(ctx context.Context, tx *domain.BankTransaction, categoryID, accountID string) error
}
