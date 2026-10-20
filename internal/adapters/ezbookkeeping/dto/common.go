package dto

// APIResponse — успішна відповідь API (обгортка над будь-якими даними).
type APIResponse[T any] struct {
	Result  T    `json:"result"`  // Дані відповіді (структура залежить від endpoint)
	Success bool `json:"success"` // Завжди true для успішної відповіді
}

// APIErrorResponse — відповідь API у разі помилки.
type APIErrorResponse struct {
	ErrorCode    int    `json:"errorCode"`    // Код помилки
	ErrorMessage string `json:"errorMessage"` // Повідомлення про помилку
	Path         string `json:"path"`         // Повний шлях API (/api/v1/{API_PATH})
	Success      bool   `json:"success"`      // Завжди false для помилки
}
