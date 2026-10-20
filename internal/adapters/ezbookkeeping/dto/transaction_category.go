package dto

// TransactionCategoryType — тип категорії транзакції.
type TransactionCategoryType int

// Константи для можливих типів категорій транзакцій згідно зі специфікацією:
const (
	TransactionCategoryTypeIncome   TransactionCategoryType = 1 // Дохід
	TransactionCategoryTypeExpense  TransactionCategoryType = 2 // Витрата
	TransactionCategoryTypeTransfer TransactionCategoryType = 3 // Переказ
)

// TransactionCategoryInfoResponse — DTO з інформацією про категорію транзакції.
type TransactionCategoryInfoResponse struct {
	ID            string                            `json:"id"`            // ID категорії транзакції
	Name          string                            `json:"name"`          // Назва категорії транзакції
	ParentID      string                            `json:"parentId"`      // ID батьківської категорії транзакції
	Type          TransactionCategoryType           `json:"type"`          // Тип категорії транзакції (1: дохід, 2: витрата, 3: переказ)
	Icon          string                            `json:"icon"`          // ID іконки категорії
	IconType      int                               `json:"iconType"`      // Тип іконки (0: системна, 1: користувацька)
	Color         string                            `json:"color"`         // Колір іконки у форматі Hex (RRGGBB)
	Comment       string                            `json:"comment"`       // Опис категорії
	DisplayOrder  int                               `json:"displayOrder"`  // Порядок відображення категорії
	Hidden        bool                              `json:"hidden"`        // Чи прихована категорія
	SubCategories []TransactionCategoryInfoResponse `json:"subCategories"` // Список вкладених підкатегорій
}

// TransactionCategoryListResponse — відповідь GET transaction/categories/list.json.
// Ключ мапи — тип категорії (1, 2, 3), значення — список категорій цього типу.
type TransactionCategoryListResponse map[TransactionCategoryType][]TransactionCategoryInfoResponse

// TransactionCategoryCreateRequest — DTO для створення нової категорії транзакції.
// Відповідь POST transaction/categories/add.json: TransactionCategoryInfoResponse.
type TransactionCategoryCreateRequest struct {
	Name     string                  `json:"name"`               // Required: Назва категорії транзакції
	Type     TransactionCategoryType `json:"type"`               // Required: Тип категорії транзакції (1: дохід, 2: витрата, 3: переказ)
	ParentID string                  `json:"parentId,omitempty"` // Optional: ID батьківської категорії ("0" для категорії верхнього рівня)
	Icon     string                  `json:"icon"`               // Required: ID іконки категорії
	IconType int                     `json:"iconType,omitempty"` // Optional: Тип іконки (0: системна, 1: користувацька; за замовчуванням 0)
	Color    string                  `json:"color"`              // Required: Колір іконки у форматі Hex (RRGGBB)
	Comment  string                  `json:"comment,omitempty"`  // Optional: Опис категорії
}
