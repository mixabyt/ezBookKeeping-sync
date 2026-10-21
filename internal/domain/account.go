package domain

import "errors"

// ErrInvalidAccountMapping — у мапінгу бракує обовʼязкового поля.
var ErrInvalidAccountMapping = errors.New("потрібні source, bank_account_id і ezb_account_id")

// AccountMapping — рахунок у банку → рахунок ezBookkeeping.
// Дефолтного рахунку немає навмисно: запис не на той рахунок ламає баланс.
type AccountMapping struct {
	Source        Source
	BankAccountID string
	EzbAccountID  string
	Name          string // для людини: "Чорна картка UAH"
}

// Validate — без будь-якого з трьох полів мапінг безглуздий: джоба однаково не знайде рахунок.
func (m AccountMapping) Validate() error {
	if m.Source == "" || m.BankAccountID == "" || m.EzbAccountID == "" {
		return ErrInvalidAccountMapping
	}
	return nil
}

// EzAccount — рахунок ezBookkeeping у плоскому вигляді, довідник для UI.
// Саме його ID записується в account_mappings.ezb_account_id.
type EzAccount struct {
	ID         string
	Name       string
	ParentName string // для субрахунків: "Мультивалютна / USD"
	Currency   string
	Hidden     bool
}

// WaitingAccount — банківський рахунок без мапінгу, що блокує джоби.
type WaitingAccount struct {
	Source        Source
	BankAccountID string
	WaitingJobs   int
	SampleDesc    string // опис однієї з транзакцій — щоб упізнати картку
}
