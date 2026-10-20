package dto

// LatestExchangeRate — курс однієї валюти.
type LatestExchangeRate struct {
	Currency string `json:"currency"` // Код валюти
	Rate     string `json:"rate"`     // Кількість одиниць цієї валюти, що дорівнює одній одиниці базової валюти
}

// LatestExchangeRateResponse — відповідь GET exchange_rates/latest.json.
type LatestExchangeRateResponse struct {
	DataSource    string               `json:"dataSource"`    // Назва джерела даних курсів валют
	ReferenceURL  string               `json:"referenceUrl"`  // Посилання на джерело даних курсів валют
	UpdateTime    int64                `json:"updateTime"`    // Unix-час оновлення даних
	BaseCurrency  string               `json:"baseCurrency"`  // Код базової валюти
	ExchangeRates []LatestExchangeRate `json:"exchangeRates"` // Список курсів валют
}

// UserCustomExchangeRateUpdateRequest — тіло запиту POST exchange_rates/user_custom/update.json.
type UserCustomExchangeRateUpdateRequest struct {
	Currency string `json:"currency"` // Required: Код валюти
	Rate     string `json:"rate"`     // Required: Кількість одиниць цієї валюти, що дорівнює одній одиниці валюти користувача за замовчуванням
}

// UserCustomExchangeRateUpdateResponse — відповідь на оновлення користувацького курсу валюти.
type UserCustomExchangeRateUpdateResponse struct {
	Currency   string `json:"currency"`   // Код валюти
	Rate       string `json:"rate"`       // Кількість одиниць цієї валюти, що дорівнює одній одиниці валюти користувача за замовчуванням
	UpdateTime int64  `json:"updateTime"` // Unix-час оновлення даних
}

// UserCustomExchangeRateDeleteRequest — тіло запиту POST exchange_rates/user_custom/delete.json. Відповідь: bool.
type UserCustomExchangeRateDeleteRequest struct {
	Currency string `json:"currency"` // Required (за змістом; у документації таблиця некоректна): Код валюти
}
