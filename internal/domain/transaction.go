package domain

import (
	"strings"
	"time"
)

// BankTransaction — транзакція з банку, незалежна від формату Monobank чи ezBookkeeping.
type BankTransaction struct {
	Source      Source
	ID          string // ID транзакції в банку; унікальний лише в парі з Source
	AccountID   string
	Time        time.Time
	Amount      int64 // у копійках, мінус — витрата
	MCC         int   // 0 — невідомо; для банків без MCC заповнюється нормалізацією
	Description string
	Hold        bool

	// BankCategory — категорія від банку для банків без MCC ("Супермаркети та продукти").
	// Потрібна лише до нормалізації, в jobs не зберігається. Для Monobank порожня.
	BankCategory string
}

// Direction — витрата чи дохід, за знаком суми.
func (t BankTransaction) Direction() Direction {
	if t.Amount < 0 {
		return DirectionExpense
	}
	return DirectionIncome
}

func (t BankTransaction) GetSourceBank() Source {
	prefix, _, _ := strings.Cut(t.ID, "_")
	switch prefix + "_" {
	case MonoPrefix:
		return SourceMono
	case PrivatPrefix:
		return SourcePrivat
	}
	return ""
}
