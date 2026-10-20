package dto

// AccountCategory — категорія рахунку.
type AccountCategory int

// Константи для можливих категорій рахунків згідно зі специфікацією:
const (
	AccountCategoryCash                 AccountCategory = 1 // Готівка
	AccountCategoryCheckingAccount      AccountCategory = 2 // Поточний рахунок
	AccountCategoryCreditCard           AccountCategory = 3 // Кредитна картка
	AccountCategoryVirtualAccount       AccountCategory = 4 // Віртуальний рахунок
	AccountCategoryDebtAccount          AccountCategory = 5 // Борговий рахунок
	AccountCategoryReceivables          AccountCategory = 6 // Дебіторська заборгованість
	AccountCategoryInvestmentAccount    AccountCategory = 7 // Інвестиційний рахунок
	AccountCategorySavingsAccount       AccountCategory = 8 // Накопичувальний рахунок
	AccountCategoryCertificateOfDeposit AccountCategory = 9 // Депозитний сертифікат
)

// AccountType — тип рахунку.
type AccountType int

// Константи для можливих типів рахунків згідно зі специфікацією:
const (
	AccountTypeSingleAccount    AccountType = 1 // Одиночний рахунок
	AccountTypeMultiSubAccounts AccountType = 2 // Рахунок з кількома субрахунками
)

// AccountInfoResponse — інформація про рахунок. GET accounts/list.json повертає []AccountInfoResponse.
type AccountInfoResponse struct {
	ID                      string                `json:"id"`                      // ID рахунку
	Name                    string                `json:"name"`                    // Назва рахунку
	ParentID                string                `json:"parentId"`                // ID батьківського рахунку
	Category                AccountCategory       `json:"category"`                // Категорія рахунку (1-9)
	Type                    AccountType           `json:"type"`                    // Тип рахунку (1: одиночний, 2: з субрахунками)
	Icon                    string                `json:"icon"`                    // ID іконки рахунку
	IconType                int                   `json:"iconType"`                // Тип іконки (0: системна, 1: користувацька)
	Color                   string                `json:"color"`                   // Колір іконки у форматі Hex (RRGGBB)
	Currency                string                `json:"currency"`                // Код валюти
	Balance                 string                `json:"balance"`                 // Баланс у мінімальних одиницях (1234 = 12.34)
	Comment                 string                `json:"comment"`                 // Опис рахунку
	CreditCardStatementDate int                   `json:"creditCardStatementDate"` // День виписки кредитної картки
	CreditCardLimit         string                `json:"creditCardLimit"`         // Кредитний ліміт у мінімальних одиницях (1234 = 12.34)
	DisplayOrder            int                   `json:"displayOrder"`            // Порядок відображення рахунку
	IsAsset                 bool                  `json:"isAsset"`                 // Чи є рахунок активом
	IsLiability             bool                  `json:"isLiability"`             // Чи є рахунок зобов'язанням
	Hidden                  bool                  `json:"hidden"`                  // Чи прихований рахунок
	SubAccounts             []AccountInfoResponse `json:"subAccounts"`             // Список субрахунків
}

// AccountCreateRequest — тіло запиту POST accounts/add.json. Відповідь: AccountInfoResponse.
// Потребує заголовки X-Timezone-Name та X-Timezone-Offset.
type AccountCreateRequest struct {
	Name                    string                 `json:"name"`                              // Required: Назва рахунку
	Category                AccountCategory        `json:"category"`                          // Required: Категорія рахунку (1-9)
	Type                    AccountType            `json:"type"`                              // Required: Тип рахунку (1: одиночний, 2: з субрахунками)
	Icon                    string                 `json:"icon"`                              // Required: ID іконки
	IconType                int                    `json:"iconType,omitempty"`                // Optional: Тип іконки (0: системна, 1: користувацька; за замовчуванням 0)
	Color                   string                 `json:"color"`                             // Required: Колір іконки у форматі Hex (RRGGBB)
	Currency                string                 `json:"currency"`                          // Required: Код валюти ISO 4217 ("---" для батьківського рахунку)
	Balance                 *string                `json:"balance,omitempty"`                 // Optional: Баланс у мінімальних одиницях (1234 = 12.34); для зобов'язань від'ємний
	BalanceTime             *int64                 `json:"balanceTime,omitempty"`             // Optional: Unix-час балансу (обов'язковий, якщо вказано balance)
	Comment                 string                 `json:"comment,omitempty"`                 // Optional: Опис рахунку
	CreditCardStatementDate *int                   `json:"creditCardStatementDate,omitempty"` // Optional: День виписки кредитної картки
	CreditCardLimit         *string                `json:"creditCardLimit,omitempty"`         // Optional: Кредитний ліміт у мінімальних одиницях (1234 = 12.34)
	SubAccounts             []AccountCreateRequest `json:"subAccounts,omitempty"`             // Optional: Список субрахунків
}
