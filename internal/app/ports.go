package app

import (
	"context"

	"ezbkTools/internal/domain"
)

type JobRepository interface {
	Add(ctx context.Context, tx *domain.BankTransaction) error
	FetchPending(ctx context.Context, limit int) ([]domain.Job, error)
	// MarkDone запамʼятовує категорію, з якою джоба пішла в ezBookkeeping:
	// з неї береться пропозиція «як минулого разу» в режимі manual.
	MarkDone(ctx context.Context, id int64, categoryID string) error
	MarkFailed(ctx context.Context, id int64, err error) error

	// MarkWaiting — для джоби бракує мапінгу; reason пояснює, чого саме.
	MarkWaiting(ctx context.Context, id int64, reason string) error
	// MarkNeedsReview — режим manual: чекаємо підтвердження категорії людиною.
	MarkNeedsReview(ctx context.Context, id int64) error
	// FetchWaiting — waiting_mapping і needs_review, з курсором afterID:
	// набір таких джоб не зменшується від обробки.
	FetchWaiting(ctx context.Context, afterID int64, limit int) ([]domain.Job, error)
	// WaitingCounts / WaitingAccounts / WaitingMerchants — що саме блокує джоби, для UI.
	WaitingCounts(ctx context.Context) (map[domain.WaitingKey]int, error)
	WaitingAccounts(ctx context.Context) ([]domain.WaitingAccount, error)
	WaitingMerchants(ctx context.Context) ([]domain.WaitingMerchant, error)
	ListNeedsReview(ctx context.Context) ([]domain.Job, error)
	// Confirm — needs_review → pending з обраною категорією.
	// Джоба вже не на підтвердженні — ErrJobNotReviewable.
	Confirm(ctx context.Context, id int64, categoryID string) error
	// Skip — needs_review → skipped; повертає, скільки джоб реально пропущено
	// (решту вже підтвердили чи обробили).
	Skip(ctx context.Context, ids []int64) (int64, error)
}

// SettingsRepository — налаштування, що змінюються з PWA.
type SettingsRepository interface {
	MappingMode(ctx context.Context) (domain.MappingMode, error)
	SetMappingMode(ctx context.Context, mode domain.MappingMode) error
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
	EzCategoryRuleByMCC(ctx context.Context, mcc int, direction domain.Direction) (*domain.MCCCategoryRule, error)
	MccByBankCategory(ctx context.Context, source domain.Source, bankCategory string) (*domain.BankCategoryMCC, error)
	// TrackUnmapped запам'ятовує ключ, для якого не знайшлося мапінгу,
	// щоб було видно, яке правило варто дописати. Повторний ключ лише збільшує лічильник.
	TrackUnmapped(ctx context.Context, key domain.UnmappedKey) error
	ListUnmapped(ctx context.Context, filter domain.UnmappedFilter) ([]domain.UnmappedKey, error)
	AddRule(ctx context.Context, rule domain.MCCCategoryRule) (*domain.MCCCategoryRule, error)
	ListRules(ctx context.Context) ([]domain.MCCCategoryRule, error)
	// UpdateRule міняє лише категорію, ознаку enabled і нотатку: діапазон і напрямок —
	// це вже інше правило. Немає такого id — ErrRuleNotFound.
	UpdateRule(ctx context.Context, id int64, categoryID string, enabled bool, note string) (*domain.MCCCategoryRule, error)

	// MerchantRule — увімкнене правило продавця; nil, nil — немає.
	MerchantRule(ctx context.Context, mcc int, direction domain.Direction, description string) (*domain.MerchantRule, error)
	AddMerchantRule(ctx context.Context, rule domain.MerchantRule) (*domain.MerchantRule, error)
	ListMerchantRules(ctx context.Context) ([]domain.MerchantRule, error)
	// UpdateMerchantRule — лише категорія й enabled. Немає такого id — ErrRuleNotFound.
	UpdateMerchantRule(ctx context.Context, id int64, categoryID string, enabled bool) (*domain.MerchantRule, error)
	// LastCategoryFor — категорія, з якою цей продавець останнього разу пішов
	// в ezBookkeeping; "" — ще не було.
	LastCategoryFor(ctx context.Context, mcc int, direction domain.Direction, description string) (string, error)
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
