package httpapi

// StatementItemDTO — головна структура вебхука від Monobank для події транзакції.
type StatementItemDTO struct {
	Type string `json:"type"` // Тип події (наприклад, "StatementItem")
	Data Data   `json:"data"` // Корисне навантаження (дані транзакції та рахунку)
}

// Data містить ідентифікатор рахунку та саму транзакцію.
type Data struct {
	Account       string        `json:"account"`       // Ідентифікатор рахунку в системі Monobank
	StatementItem StatementItem `json:"statementItem"` // Детальна інформація про транзакцію
}

// StatementItem описує конкретну фінансову операцію (транзакцію).
type StatementItem struct {
	ID              string `json:"id"`              // Унікальний ідентифікатор транзакції
	Time            int64  `json:"time"`            // Час транзакції у форматі Unix timestamp
	Description     string `json:"description"`     // Опис транзакції (назва магазину, призначення платежу тощо)
	MCC             int    `json:"mcc"`             // MCC-код категорії торговельної точки
	OriginalMCC     int    `json:"originalMcc"`     // Оригінальний MCC-код (якщо відбулась конвертація чи коригування)
	Amount          int    `json:"amount"`          // Сума транзакції у мінімальних одиницях валюти (наприклад, копійки; мінусова — витрата)
	OperationAmount int    `json:"operationAmount"` // Сума у валюті самої операції (якщо відрізняється від валюти рахунку)
	CurrencyCode    int    `json:"currencyCode"`    // Цифровий код валюти згідно з ISO 4217 (наприклад, 980 для UAH)
	CommissionRate  int    `json:"commissionRate"`  // Сума комісії банку
	CashbackAmount  int    `json:"cashbackAmount"`  // Сума нарахованого кешбеку у мінімальних одиницях (копійках)
	Balance         int    `json:"balance"`         // Залишкова сума на рахунку після проведення транзакції (у копійках)
	Hold            bool   `json:"hold"`            // Статус холду: true — сума заблокована (не проведені гроші), false — проведені
	ReceiptID       string `json:"receiptId"`       // Ідентифікатор фіскального чека (якщо сформований)
}
