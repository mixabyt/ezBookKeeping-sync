package domain

import (
	"errors"
	"fmt"
	"time"
)

// Помилки валідації правила — перевіряються до БД, щоб UI отримав
// зрозуміле повідомлення, а не "constraint failed".
var (
	ErrInvalidMCCRange  = errors.New("MCC має бути в межах 0–9999, а mcc_from не більший за mcc_to")
	ErrInvalidDirection = errors.New("direction має бути 'expense' або 'income'")
	ErrEmptyCategory    = errors.New("не вказана категорія ezBookkeeping")
)

// BankCategoryMCC — крок 1 категоризації (нормалізація): банківська категорія → MCC.
// Для банків без MCC (напр. Приват); для Monobank не потрібна.
type BankCategoryMCC struct {
	Source       Source
	BankCategory string
	MCC          int
}

// MCCCategoryRule — крок 2 категоризації: MCC + напрямок → категорія ezBookkeeping.
// Однакове для всіх банків. Один MCC — це MCCFrom == MCCTo.
// З кількох підхожих правил перемагає те, в кого менший Priority.
type MCCCategoryRule struct {
	ID            int64
	MCCFrom       int
	MCCTo         int
	Direction     Direction
	Priority      int
	EzbCategoryID string
	Enabled       bool
	Note          string // для людини: "ресторани й кафе"
}

// Validate повторює обмеження зі схеми mcc_category_rules:
// краще віддати 400 з поясненням, ніж 500 від CHECK-констрейнта.
func (r MCCCategoryRule) Validate() error {
	if r.MCCFrom < 0 || r.MCCTo > 9999 || r.MCCFrom > r.MCCTo {
		return fmt.Errorf("%w: отримано %d–%d", ErrInvalidMCCRange, r.MCCFrom, r.MCCTo)
	}
	if r.Direction != DirectionExpense && r.Direction != DirectionIncome {
		return fmt.Errorf("%w: отримано %q", ErrInvalidDirection, r.Direction)
	}
	if r.EzbCategoryID == "" {
		return ErrEmptyCategory
	}
	return nil
}

// EzCategoryKind — до чого категорія ezBookkeeping придатна.
// Для правил MCC цікаві лише income/expense, transfer — довідково.
type EzCategoryKind string

const (
	EzCategoryIncome   EzCategoryKind = "income"
	EzCategoryExpense  EzCategoryKind = "expense"
	EzCategoryTransfer EzCategoryKind = "transfer"
)

// KindFor — яка категорія підходить для цього напрямку руху грошей.
func KindFor(direction Direction) EzCategoryKind {
	if direction == DirectionIncome {
		return EzCategoryIncome
	}
	return EzCategoryExpense
}

// EzCategory — категорія ezBookkeeping у плоскому вигляді: саме її id
// записується в mcc_category_rules.ezb_category_id.
type EzCategory struct {
	ID         string
	Name       string
	ParentID   string
	ParentName string // щоб у UI було видно "Транспорт / Таксі", а не просто "Таксі"
	Kind       EzCategoryKind
	Hidden     bool
}

// EzCategoryFilter — параметри вибірки довідника для UI.
type EzCategoryFilter struct {
	Kind          EzCategoryKind // порожній — усі
	IncludeHidden bool
}

// KeyType — на якому кроці категоризації не знайшовся мапінг.
type KeyType string

const (
	KeyBankCategory KeyType = "bank_category" // крок 1: немає MCC для банківської категорії
	KeyMCC          KeyType = "mcc"           // крок 2: немає правила для MCC
)

// UnmappedFilter — параметри вибірки unmapped-ключів для UI.
type UnmappedFilter struct {
	KeyType     KeyType // порожній — усі типи
	OnlyPending bool    // лише ті, для яких правила досі немає
	Limit       int
}

// UnmappedKey — ключ без мапінгу; підказка, що варто дописати.
type UnmappedKey struct {
	Source     Source
	KeyType    KeyType
	Key        string // "5411" або "Супермаркети та продукти"
	Direction  Direction
	SampleDesc string
	Hits       int
	FirstSeen  time.Time
	LastSeen   time.Time

	// Resolved — чи зʼявилося відтоді правило для цього ключа.
	// Рядки з unmapped_keys не видаляються, тому без цієї ознаки список
	// ніколи б не порожнів: він показує історію, а не лише борг.
	Resolved bool
}

// Defaults — чим заповнювати транзакцію, коли мапінгу немає: категорія, коли жодне
// правило в mcc_category_rules не підійшло, і рахунок, коли немає рядка в account_mappings.
// Беруться з конфігу: ID специфічні для інстансу ezBookkeeping.
// Порожнє значення означає «дефолту немає» — тоді транзакцію не записуємо.
type Defaults struct {
	ExpenseCategoryID string
	IncomeCategoryID  string
	SourceAccountID   string
}

// CategoryFor — дефолтна категорія для напрямку руху грошей.
func (d Defaults) CategoryFor(direction Direction) string {
	if direction == DirectionIncome {
		return d.IncomeCategoryID
	}
	return d.ExpenseCategoryID
}
