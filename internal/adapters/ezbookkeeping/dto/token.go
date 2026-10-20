package dto

// TokenType — тип токена.
type TokenType int

// Константи для можливих типів токенів згідно зі специфікацією:
const (
	TokenTypeNormal TokenType = 1 // Звичайний токен
	TokenTypeMCP    TokenType = 5 // MCP-токен
	TokenTypeAPI    TokenType = 8 // API-токен
)

// TokenInfoResponse — інформація про сесію (токен). GET tokens/list.json повертає []TokenInfoResponse.
type TokenInfoResponse struct {
	TokenID   string    `json:"tokenId"`   // ID токена
	TokenType TokenType `json:"tokenType"` // Тип токена (1: звичайний, 5: MCP, 8: API)
	UserAgent string    `json:"userAgent"` // User-Agent, збережений при створенні сесії
	LastSeen  int64     `json:"lastSeen"`  // Unix-час останнього оновлення сесії
	IsCurrent bool      `json:"isCurrent"` // Чи є це поточною сесією
}

// TokenRevokeRequest — тіло запиту POST tokens/revoke.json. Відповідь: bool (чи успішно відкликано).
type TokenRevokeRequest struct {
	TokenID string `json:"tokenId"` // Required: ID токена, який потрібно відкликати
}
