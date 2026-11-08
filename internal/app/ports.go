package app

import (
	"context"
	"time"

	"ezbkTools/internal/domain"
)

// Майже всі методи приймають userID явним параметром, а не через context:
// так компілятор показує кожне місце, де про користувача забули.

type JobRepository interface {
	Add(ctx context.Context, userID int64, tx *domain.BankTransaction) error
	// FetchPending і FetchWaiting — для воркера: він іде по всіх користувачах,
	// а чия джоба, видно з Job.UserID.
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
	WaitingCounts(ctx context.Context, userID int64) (map[domain.WaitingKey]int, error)
	WaitingAccounts(ctx context.Context, userID int64) ([]domain.WaitingAccount, error)
	WaitingMerchants(ctx context.Context, userID int64) ([]domain.WaitingMerchant, error)
	ListNeedsReview(ctx context.Context, userID int64) ([]domain.Job, error)
	// Confirm — needs_review → pending з обраною категорією.
	// Джоба вже не на підтвердженні або чужа — ErrJobNotReviewable.
	Confirm(ctx context.Context, userID, id int64, categoryID string) error
	// Skip — needs_review → skipped; повертає, скільки джоб реально пропущено
	// (решту вже підтвердили чи обробили, а чужі просто не підпадають під умову).
	Skip(ctx context.Context, userID int64, ids []int64) (int64, error)
}

// SettingsRepository — налаштування, що змінюються з PWA.
type SettingsRepository interface {
	MappingMode(ctx context.Context, userID int64) (domain.MappingMode, error)
	SetMappingMode(ctx context.Context, userID int64, mode domain.MappingMode) error
}

// AccountRepository — мапінг банківського рахунку на рахунок ezBookkeeping.
// Дефолту немає навмисно: запис не на той рахунок ламає баланс.
type AccountRepository interface {
	EzAccountByBank(ctx context.Context, userID int64, source domain.Source, bankAccountID string) (*domain.AccountMapping, error)
	AddMapping(ctx context.Context, userID int64, mapping domain.AccountMapping) error
	ListMappings(ctx context.Context, userID int64) ([]domain.AccountMapping, error)
}

// JobWaker — розбудити воркер, щоб він одразу перевірив джоби, а не чекав тіку.
// Окремий порт, щоб MappingService не залежав від SyncService.
type JobWaker interface {
	Wake()
}

// CategoryRepository — мапінг MCC → категорія ezBookkeeping (зберігається в БД).
type CategoryRepository interface {
	EzCategoryRuleByMCC(ctx context.Context, userID int64, mcc int, direction domain.Direction) (*domain.MCCCategoryRule, error)
	// MccByBankCategory без userID: це довідник банку, спільний для всіх.
	MccByBankCategory(ctx context.Context, source domain.Source, bankCategory string) (*domain.BankCategoryMCC, error)
	// TrackUnmapped запам'ятовує ключ, для якого не знайшлося мапінгу,
	// щоб було видно, яке правило варто дописати. Повторний ключ лише збільшує лічильник.
	TrackUnmapped(ctx context.Context, userID int64, key domain.UnmappedKey) error
	ListUnmapped(ctx context.Context, userID int64, filter domain.UnmappedFilter) ([]domain.UnmappedKey, error)
	AddRule(ctx context.Context, userID int64, rule domain.MCCCategoryRule) (*domain.MCCCategoryRule, error)
	ListRules(ctx context.Context, userID int64) ([]domain.MCCCategoryRule, error)
	// UpdateRule міняє лише категорію, ознаку enabled і нотатку: діапазон і напрямок —
	// це вже інше правило. Немає такого id у цього користувача — ErrRuleNotFound.
	UpdateRule(ctx context.Context, userID, id int64, categoryID string, enabled bool, note string) (*domain.MCCCategoryRule, error)

	// MerchantRule — увімкнене правило продавця; nil, nil — немає.
	MerchantRule(ctx context.Context, userID int64, mcc int, direction domain.Direction, description string) (*domain.MerchantRule, error)
	AddMerchantRule(ctx context.Context, userID int64, rule domain.MerchantRule) (*domain.MerchantRule, error)
	ListMerchantRules(ctx context.Context, userID int64) ([]domain.MerchantRule, error)
	// UpdateMerchantRule — лише категорія й enabled. Немає такого id у цього користувача — ErrRuleNotFound.
	UpdateMerchantRule(ctx context.Context, userID, id int64, categoryID string, enabled bool) (*domain.MerchantRule, error)
	// LastCategoryFor — категорія, з якою цей продавець останнього разу пішов
	// в ezBookkeeping; "" — ще не було.
	LastCategoryFor(ctx context.Context, userID int64, mcc int, direction domain.Direction, description string) (string, error)
}

// UserRepository — користувачі. Методи читання повертають nil, nil, якщо такого немає.
type UserRepository interface {
	ByID(ctx context.Context, id int64) (*domain.User, error)
	ByLogin(ctx context.Context, login string) (*domain.User, error)
	ByWebhookToken(ctx context.Context, token string) (*domain.User, error)
	// Create — логін зайнятий: ErrLoginTaken.
	Create(ctx context.Context, user domain.User) (*domain.User, error)
	// FillOwner заповнює порожній рядок власника даними з .env; якщо хеш уже є — нічого не робить.
	FillOwner(ctx context.Context, login, passwordHash, ezbToken string) error
	// SetPassword міняє хеш, знімає MustChangePassword і піднімає SessionVersion;
	// повертає нову версію сесії.
	SetPassword(ctx context.Context, id int64, passwordHash string) (int64, error)
	SetEzbToken(ctx context.Context, id int64, token string) error
	SetMonoWebhookRegistered(ctx context.Context, id int64, at time.Time) error
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

// Ledger — акаунт ezBookkeeping одного користувача.
type Ledger interface {
	Bookkeeper
	CategoryCatalog
}

// LedgerFactory дає клієнт ezBookkeeping для токена конкретного користувача.
type LedgerFactory func(token string) Ledger

// MonoWebhookRegistrar реєструє адресу вебхука в Monobank особистим токеном клієнта.
type MonoWebhookRegistrar interface {
	// SetWebhook повертає ErrBadMonoToken, ErrMonoRateLimited, ErrMonoRejected або ErrUpstreamUnavailable.
	SetWebhook(ctx context.Context, monoToken, webhookURL string) error
}
