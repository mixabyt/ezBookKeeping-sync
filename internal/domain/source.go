package domain

// Source — банк, з якого прийшла транзакція.
// Без закритого списку в БД: новий банк — це нова константа, а не міграція.
type Source string

const (
	SourceMono   Source = "mono"
	SourcePrivat Source = "privat"
	MonoPrefix   string = "mb_"
	PrivatPrefix string = "pb_"
)

// Direction — напрямок руху грошей.
type Direction string

const (
	DirectionExpense Direction = "expense"
	DirectionIncome  Direction = "income"
)
