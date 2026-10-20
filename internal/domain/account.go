package domain

// AccountMapping — рахунок у банку → рахунок ezBookkeeping.
// Дефолтного рахунку немає навмисно: запис не на той рахунок ламає баланс.
type AccountMapping struct {
	Source        Source
	BankAccountID string
	EzbAccountID  string
	Name          string // для людини: "Чорна картка UAH"
}
