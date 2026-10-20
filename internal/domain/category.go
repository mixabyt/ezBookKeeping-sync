package domain

import "time"

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

// KeyType — на якому кроці категоризації не знайшовся мапінг.
type KeyType string

const (
	KeyBankCategory KeyType = "bank_category" // крок 1: немає MCC для банківської категорії
	KeyMCC          KeyType = "mcc"           // крок 2: немає правила для MCC
)

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
}
