package app

import (
	"context"
	"errors"

	"ezbkTools/internal/domain"
)

// ErrRuleExists — правило з таким діапазоном і напрямком уже є.
// Окрема помилка, щоб адаптер віддав 409, а не 500.
var ErrRuleExists = errors.New("правило для цього діапазону MCC і напрямку вже існує")

// MappingService — сценарії ручного мапінгу: що лишилось замапити і (далі) створення правил.
// Окремо від SyncService навмисно: там гарячий шлях транзакції, тут — робота людини через UI.
type MappingService struct {
	categories CategoryRepository
	catalog    CategoryCatalog
}

func NewMappingService(categories CategoryRepository, catalog CategoryCatalog) *MappingService {
	return &MappingService{categories: categories, catalog: catalog}
}

// EzCategories — довідник категорій ezBookkeeping для випадайки в UI.
// Приховані категорії за замовчуванням не віддаємо: пропонувати до вибору те,
// що людина свідомо сховала в ezBookkeeping, сенсу немає.
func (s *MappingService) EzCategories(ctx context.Context, filter domain.EzCategoryFilter) ([]domain.EzCategory, error) {
	all, err := s.catalog.Categories(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]domain.EzCategory, 0, len(all))
	for _, category := range all {
		if category.Hidden && !filter.IncludeHidden {
			continue
		}
		if filter.Kind != "" && category.Kind != filter.Kind {
			continue
		}
		result = append(result, category)
	}
	return result, nil
}

// maxUnmappedLimit — стеля вибірки, щоб один запит з UI не витягнув усю таблицю.
const maxUnmappedLimit = 500

// CreateRule додає правило MCC → категорія. Після цього ключ у unmapped_keys
// автоматично стає resolved: список їх звіряє на льоту, нічого синхронізувати не треба.
func (s *MappingService) CreateRule(ctx context.Context, rule domain.MCCCategoryRule) (*domain.MCCCategoryRule, error) {
	if rule.Priority <= 0 {
		rule.Priority = defaultRulePriority
	}
	if err := rule.Validate(); err != nil {
		return nil, err
	}
	return s.categories.AddRule(ctx, rule)
}

// defaultRulePriority — як у схемі: з кількох підхожих правил виграє менший priority,
// тож точкові правила варто додавати з меншим числом, ніж діапазонні.
const defaultRulePriority = 100

// UnmappedKeys — ключі, які зустрічались у транзакціях без мапінгу,
// найчастіші першими: саме їх має сенс мапити в першу чергу.
func (s *MappingService) UnmappedKeys(ctx context.Context, filter domain.UnmappedFilter) ([]domain.UnmappedKey, error) {
	if filter.Limit <= 0 || filter.Limit > maxUnmappedLimit {
		filter.Limit = maxUnmappedLimit
	}
	return s.categories.ListUnmapped(ctx, filter)
}
