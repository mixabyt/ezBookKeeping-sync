package dto

// TransactionType — тип транзакції.
type TransactionType int

// Константи для можливих типів транзакцій згідно зі специфікацією:
const (
	TransactionTypeBalanceModification TransactionType = 1 // Зміна балансу
	TransactionTypeIncome              TransactionType = 2 // Дохід
	TransactionTypeExpense             TransactionType = 3 // Витрата
	TransactionTypeTransfer            TransactionType = 4 // Переказ
)

// TransactionGeoLocationRequest описує географічні координати транзакції.
type TransactionGeoLocationRequest struct {
	Latitude  *float64 `json:"latitude,omitempty"`  // Optional: Широта
	Longitude *float64 `json:"longitude,omitempty"` // Optional: Довгота
}

// TransactionCreateRequest — головний DTO для створення нової транзакції (POST transactions/add.json).
type TransactionCreateRequest struct {
	Type                 TransactionType                `json:"type"`                           // Required: Тип транзакції
	CategoryID           string                         `json:"categoryId"`                     // Required: ID категорії транзакції (вторинна категорія)
	Time                 int64                          `json:"time"`                           // Required: Час транзакції (Unix timestamp)
	UtcOffset            int                            `json:"utcOffset"`                      // Required: Зміщення часового поясу (у хвилинах)
	SourceAccountId      string                         `json:"sourceAccountId"`                // Required: ID вихідного рахунку
	DestinationAccountId *string                        `json:"destinationAccountId,omitempty"` // Optional: ID цільового рахунку (для переказів)
	SourceAmount         int                            `json:"sourceAmount"`                   // Required: Сума списання (до 2 знаків після коми: 1234 = 12.34)
	DestinationAmount    *int                           `json:"destinationAmount,omitempty"`    // Optional: Сума зарахування
	HideAmount           *bool                          `json:"hideAmount,omitempty"`           // Optional: Чи приховувати суму
	TagIds               []string                       `json:"tagIds,omitempty"`               // Optional: Масив ID тегів
	PictureIds           []string                       `json:"pictureIds,omitempty"`           // Optional: Масив ID картинок/чеків
	Comment              *string                        `json:"comment,omitempty"`              // Optional: Опис / коментар до транзакції
	GeoLocation          *TransactionGeoLocationRequest `json:"geoLocation,omitempty"`          // Optional: Географічна локація
}

// TransactionPictureInfoBasicResponse — базова інформація про зображення транзакції.
type TransactionPictureInfoBasicResponse struct {
	PictureID   string `json:"pictureId"`   // ID зображення транзакції
	OriginalURL string `json:"originalUrl"` // URL зображення транзакції
}

// TransactionGeoLocationResponse — географічна локація транзакції у відповіді.
type TransactionGeoLocationResponse struct {
	Latitude  float64 `json:"latitude"`  // Широта
	Longitude float64 `json:"longitude"` // Довгота
}

// TransactionInfoResponse — інформація про транзакцію.
// Якщо у запиті вказано trim_*, відповідні об'єкти (category, sourceAccount, ...) можуть бути відсутні — замість них повертаються ID.
type TransactionInfoResponse struct {
	ID                   string                                `json:"id"`                   // ID транзакції
	TimeSequenceID       string                                `json:"timeSequenceId"`       // ID часової послідовності транзакції
	Type                 TransactionType                       `json:"type"`                 // Тип транзакції (1: зміна балансу, 2: дохід, 3: витрата, 4: переказ)
	CategoryID           string                                `json:"categoryId"`           // ID категорії транзакції
	Category             *TransactionCategoryInfoResponse      `json:"category"`             // Об'єкт категорії транзакції
	Time                 int64                                 `json:"time"`                 // Unix-час транзакції
	UtcOffset            int                                   `json:"utcOffset"`            // Зміщення часового поясу транзакції (у хвилинах)
	SourceAccountID      string                                `json:"sourceAccountId"`      // ID вихідного рахунку
	SourceAccount        *AccountInfoResponse                  `json:"sourceAccount"`        // Об'єкт вихідного рахунку
	DestinationAccountID string                                `json:"destinationAccountId"` // ID цільового рахунку
	DestinationAccount   *AccountInfoResponse                  `json:"destinationAccount"`   // Об'єкт цільового рахунку
	SourceAmount         int64                                 `json:"sourceAmount"`         // Сума списання (1234 = 12.34)
	DestinationAmount    int64                                 `json:"destinationAmount"`    // Сума зарахування (1234 = 12.34)
	HideAmount           bool                                  `json:"hideAmount"`           // Чи приховувати суму
	TagIDs               []string                              `json:"tagIds"`               // Масив ID тегів транзакції
	Tags                 []TransactionTagInfoResponse          `json:"tags"`                 // Масив об'єктів тегів транзакції
	Pictures             []TransactionPictureInfoBasicResponse `json:"pictures"`             // Список зображень транзакції
	Comment              string                                `json:"comment"`              // Опис транзакції
	GeoLocation          *TransactionGeoLocationResponse       `json:"geoLocation"`          // Географічна локація транзакції
	Editable             bool                                  `json:"editable"`             // Чи можна редагувати транзакцію
}

// TransactionInfoPageWrapperResponse — відповідь GET transactions/list.json (посторінкове отримання).
type TransactionInfoPageWrapperResponse struct {
	Items              []TransactionInfoResponse `json:"items"`              // Список транзакцій
	NextTimeSequenceID int64                     `json:"nextTimeSequenceId"` // Наступний курсор (значення max_time) для запиту старіших даних
	TotalCount         int                       `json:"totalCount"`         // Загальна кількість транзакцій
}

// TransactionListRequest — query-параметри GET transactions/list.json.
// Потребує заголовки X-Timezone-Name та X-Timezone-Offset. Теги `url` — для github.com/google/go-querystring.
type TransactionListRequest struct {
	Type             *TransactionType `url:"type,omitempty"`               // Optional: Фільтр за типом транзакції (1: зміна балансу, 2: дохід, 3: витрата, 4: переказ)
	CategoryIDs      string           `url:"category_ids,omitempty"`       // Optional: ID категорій через кому
	AccountIDs       string           `url:"account_ids,omitempty"`        // Optional: ID рахунків через кому
	TagFilter        string           `url:"tag_filter,omitempty"`         // Optional: Фільтр тегів; групи через ";", кожна група "type:tag_id1,tag_id2" (0: усі теги групи, 1: будь-який тег групи, 2: не всі теги групи, 3: жодного тега групи)
	AmountFilter     string           `url:"amount_filter,omitempty"`      // Optional: Фільтр суми (gt:{amount}, lt:{amount}, eq:{amount}, ne:{amount}, bt:{min}:{max} — між, ne:{min}:{max} — не між, так у документації)
	Keyword          string           `url:"keyword,omitempty"`            // Optional: Фільтр за ключовим словом
	MatchMode        *int             `url:"match_mode,omitempty"`         // Optional: Режим пошуку за ключовим словом (0: налаштування БД за замовчуванням, 1: без урахування регістру)
	MustHavePictures *bool            `url:"must_have_pictures,omitempty"` // Optional: Повертати лише транзакції із зображеннями
	MaxTime          *int64           `url:"max_time,omitempty"`           // Optional: Максимальний ID часової послідовності (0 — найновіші транзакції)
	MinTime          *int64           `url:"min_time,omitempty"`           // Optional: Мінімальний ID часової послідовності
	Page             *int             `url:"page,omitempty"`               // Optional: Номер сторінки
	Count            int              `url:"count"`                        // Required: Кількість транзакцій на сторінці (максимум 50)
	WithCount        *bool            `url:"with_count,omitempty"`         // Optional: Повертати загальну кількість транзакцій
	WithPictures     *bool            `url:"with_pictures,omitempty"`      // Optional: Повертати ID зображень
	TrimAccount      *bool            `url:"trim_account,omitempty"`       // Optional: Повертати ID рахунків замість об'єктів
	TrimCategory     *bool            `url:"trim_category,omitempty"`      // Optional: Повертати ID категорії замість об'єкта
	TrimTag          *bool            `url:"trim_tag,omitempty"`           // Optional: Повертати ID тегів замість об'єктів
}

// TransactionListAllRequest — query-параметри GET transactions/list/all.json. Відповідь: []TransactionInfoResponse.
// Потребує заголовки X-Timezone-Name та X-Timezone-Offset.
type TransactionListAllRequest struct {
	Type             *TransactionType `url:"type,omitempty"`               // Optional: Фільтр за типом транзакції (1: зміна балансу, 2: дохід, 3: витрата, 4: переказ)
	CategoryIDs      string           `url:"category_ids,omitempty"`       // Optional: ID категорій через кому
	AccountIDs       string           `url:"account_ids,omitempty"`        // Optional: ID рахунків через кому
	TagFilter        string           `url:"tag_filter,omitempty"`         // Optional: Фільтр тегів (формат і значення типів 0-3 як у TransactionListRequest)
	AmountFilter     string           `url:"amount_filter,omitempty"`      // Optional: Фільтр суми (формат як у TransactionListRequest)
	Keyword          string           `url:"keyword,omitempty"`            // Optional: Фільтр за ключовим словом
	MatchMode        *int             `url:"match_mode,omitempty"`         // Optional: Режим пошуку (0: налаштування БД за замовчуванням, 1: без урахування регістру)
	MustHavePictures *bool            `url:"must_have_pictures,omitempty"` // Optional: Повертати лише транзакції із зображеннями
	StartTime        *int64           `url:"start_time,omitempty"`         // Optional: Початковий час (Unix)
	EndTime          *int64           `url:"end_time,omitempty"`           // Optional: Кінцевий час (Unix)
	WithPictures     *bool            `url:"with_pictures,omitempty"`      // Optional: Повертати ID зображень
	TrimAccount      *bool            `url:"trim_account,omitempty"`       // Optional: Повертати ID рахунків замість об'єктів
	TrimCategory     *bool            `url:"trim_category,omitempty"`      // Optional: Повертати ID категорії замість об'єкта
	TrimTag          *bool            `url:"trim_tag,omitempty"`           // Optional: Повертати ID тегів замість об'єктів
}

// TransactionModifyRequest — тіло запиту POST transactions/modify.json.
// Потребує заголовки X-Timezone-Name та X-Timezone-Offset.
type TransactionModifyRequest struct {
	ID                   string                         `json:"id"`                             // Required: ID транзакції
	Type                 TransactionType                `json:"type"`                           // Required: Тип транзакції (1: зміна балансу, 2: дохід, 3: витрата, 4: переказ)
	CategoryID           *string                        `json:"categoryId,omitempty"`           // Optional*: ID категорії (вторинна категорія); не потрібен лише для зміни балансу, для інших типів обов'язковий
	Time                 int64                          `json:"time"`                           // Required: Unix-час транзакції
	UtcOffset            int                            `json:"utcOffset"`                      // Required: Зміщення часового поясу (у хвилинах)
	SourceAccountID      string                         `json:"sourceAccountId"`                // Required: ID вихідного рахунку (без субрахунків або субрахунок)
	DestinationAccountID *string                        `json:"destinationAccountId,omitempty"` // Optional: ID цільового рахунку (без субрахунків або субрахунок)
	SourceAmount         int64                          `json:"sourceAmount"`                   // Required: Сума списання (1234 = 12.34)
	DestinationAmount    *int64                         `json:"destinationAmount,omitempty"`    // Optional: Сума зарахування (1234 = 12.34)
	HideAmount           *bool                          `json:"hideAmount,omitempty"`           // Optional: Чи приховувати суму
	TagIDs               []string                       `json:"tagIds,omitempty"`               // Optional: Масив ID тегів
	PictureIDs           []string                       `json:"pictureIds,omitempty"`           // Optional: Масив ID зображень
	Comment              *string                        `json:"comment,omitempty"`              // Optional: Опис / коментар до транзакції
	GeoLocation          *TransactionGeoLocationRequest `json:"geoLocation,omitempty"`          // Optional: Географічна локація
}

// TransactionDeleteRequest — тіло запиту POST transactions/delete.json.
// Потребує заголовки X-Timezone-Name та X-Timezone-Offset.
type TransactionDeleteRequest struct {
	ID string `json:"id"` // Required: ID транзакції
}
