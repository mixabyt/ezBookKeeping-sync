package dto

// TransactionTagInfoResponse — інформація про тег транзакції.
// GET transaction/tags/list.json повертає []TransactionTagInfoResponse.
type TransactionTagInfoResponse struct {
	ID           string `json:"id"`           // ID тега транзакції
	Name         string `json:"name"`         // Назва тега транзакції
	GroupID      string `json:"groupId"`      // ID групи тегів
	DisplayOrder int    `json:"displayOrder"` // Порядок відображення тега
	Hidden       bool   `json:"hidden"`       // Чи прихований тег
}

// TransactionTagCreateRequest — тіло запиту POST transaction/tags/add.json. Відповідь: TransactionTagInfoResponse.
type TransactionTagCreateRequest struct {
	GroupID string `json:"groupId,omitempty"` // Optional: ID групи тегів ("0" — група за замовчуванням)
	Name    string `json:"name"`              // Required: Назва тега транзакції
}
