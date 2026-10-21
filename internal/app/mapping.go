package app

import (
	"context"
	"errors"
	"strconv"

	"ezbkTools/internal/domain"
)

// ErrRuleExists — правило з таким діапазоном і напрямком уже є.
// Окрема помилка, щоб адаптер віддав 409, а не 500.
var ErrRuleExists = errors.New("правило для цього діапазону MCC і напрямку вже існує")

// MappingService — сценарії ручного мапінгу: що лишилось замапити і створення мапінгів.
// Окремо від SyncService навмисно: там гарячий шлях транзакції, тут — робота людини через UI.
// Після кожного нового мапінгу будить воркер, щоб джоби, які на нього чекали, рушили одразу.
type MappingService struct {
	categories CategoryRepository
	accounts   AccountRepository
	catalog    CategoryCatalog
	jobs       JobRepository
	waker      JobWaker
}

func NewMappingService(
	categories CategoryRepository,
	accounts AccountRepository,
	catalog CategoryCatalog,
	jobs JobRepository,
	waker JobWaker,
) *MappingService {
	return &MappingService{
		categories: categories,
		accounts:   accounts,
		catalog:    catalog,
		jobs:       jobs,
		waker:      waker,
	}
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

// EzAccounts — довідник рахунків ezBookkeeping, приховані відкидаємо з тієї ж причини.
func (s *MappingService) EzAccounts(ctx context.Context) ([]domain.EzAccount, error) {
	all, err := s.catalog.Accounts(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]domain.EzAccount, 0, len(all))
	for _, account := range all {
		if !account.Hidden {
			result = append(result, account)
		}
	}
	return result, nil
}

// maxUnmappedLimit — стеля вибірки, щоб один запит з UI не витягнув усю таблицю.
const maxUnmappedLimit = 500

// defaultRulePriority — як у схемі: з кількох підхожих правил виграє менший priority,
// тож точкові правила варто додавати з меншим числом, ніж діапазонні.
const defaultRulePriority = 100

// CreateRule додає правило MCC → категорія і будить воркер.
// queued — скільки джоб чекало саме на це правило; вони перевіряються повторно,
// але рушать далі лише якщо для них уже є й мапінг рахунку.
func (s *MappingService) CreateRule(ctx context.Context, rule domain.MCCCategoryRule) (*domain.MCCCategoryRule, int, error) {
	if rule.Priority <= 0 {
		rule.Priority = defaultRulePriority
	}
	if err := rule.Validate(); err != nil {
		return nil, 0, err
	}

	created, err := s.categories.AddRule(ctx, rule)
	if err != nil {
		return nil, 0, err
	}

	queued := 0
	if counts, err := s.jobs.WaitingCounts(ctx); err == nil {
		for key, n := range counts {
			if key.Direction == created.Direction && key.MCC >= created.MCCFrom && key.MCC <= created.MCCTo {
				queued += n
			}
		}
	}
	// лічильник — лише для UI; правило вже створено, тому його помилку не віддаємо

	s.waker.Wake()
	return created, queued, nil
}

// UnmappedKeys — ключі, які зустрічались у транзакціях без мапінгу,
// найчастіші першими, з кількістю транзакцій, які вони зараз блокують.
func (s *MappingService) UnmappedKeys(ctx context.Context, filter domain.UnmappedFilter) ([]domain.UnmappedKey, error) {
	if filter.Limit <= 0 || filter.Limit > maxUnmappedLimit {
		filter.Limit = maxUnmappedLimit
	}

	keys, err := s.categories.ListUnmapped(ctx, filter)
	if err != nil {
		return nil, err
	}

	counts, err := s.jobs.WaitingCounts(ctx)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if keys[i].KeyType != domain.KeyMCC {
			continue
		}
		mcc, err := strconv.Atoi(keys[i].Key)
		if err != nil {
			continue
		}
		keys[i].WaitingJobs = counts[domain.WaitingKey{MCC: mcc, Direction: keys[i].Direction}]
	}
	return keys, nil
}

// WaitingAccounts — банківські рахунки, через які стоять джоби, і для яких мапінгу досі немає.
// Рахунок із мапінгом тут не показуємо, навіть якщо його джоби чекають: вони чекають
// категорію, і це видно в списку MCC.
func (s *MappingService) WaitingAccounts(ctx context.Context) ([]domain.WaitingAccount, error) {
	waiting, err := s.jobs.WaitingAccounts(ctx)
	if err != nil {
		return nil, err
	}
	mappings, err := s.accounts.ListMappings(ctx)
	if err != nil {
		return nil, err
	}

	mapped := make(map[string]bool, len(mappings))
	for _, m := range mappings {
		mapped[string(m.Source)+"/"+m.BankAccountID] = true
	}

	result := make([]domain.WaitingAccount, 0, len(waiting))
	for _, w := range waiting {
		if !mapped[string(w.Source)+"/"+w.BankAccountID] {
			result = append(result, w)
		}
	}
	return result, nil
}

// CreateAccountMapping задає (або виправляє) рахунок ezBookkeeping для банківського
// рахунку і будить воркер. queued — скільки джоб із цього рахунку чекало.
func (s *MappingService) CreateAccountMapping(ctx context.Context, mapping domain.AccountMapping) (int, error) {
	if err := mapping.Validate(); err != nil {
		return 0, err
	}
	if err := s.accounts.AddMapping(ctx, mapping); err != nil {
		return 0, err
	}

	queued := 0
	if waiting, err := s.jobs.WaitingAccounts(ctx); err == nil {
		for _, w := range waiting {
			if w.Source == mapping.Source && w.BankAccountID == mapping.BankAccountID {
				queued = w.WaitingJobs
			}
		}
	}

	s.waker.Wake()
	return queued, nil
}
