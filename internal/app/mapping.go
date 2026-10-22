package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"ezbkTools/internal/domain"
)

// ErrRuleExists — правило з таким діапазоном і напрямком уже є.
// Окрема помилка, щоб адаптер віддав 409, а не 500.
var ErrRuleExists = errors.New("таке правило вже існує")

// ErrRuleNotFound — правила з таким id немає (адаптер віддає 404).
var ErrRuleNotFound = errors.New("правило не знайдено")

// ErrJobNotReviewable — джоба вже не чекає підтвердження: її підтвердили раніше
// (друга вкладка, подвійний тап) або режим змінився і вона пішла сама. Адаптер віддає 409.
var ErrJobNotReviewable = errors.New("транзакція вже не чекає підтвердження")

// MappingService — сценарії ручного мапінгу: що лишилось замапити і створення мапінгів.
// Окремо від SyncService навмисно: там гарячий шлях транзакції, тут — робота людини через UI.
// Після кожного нового мапінгу будить воркер, щоб джоби, які на нього чекали, рушили одразу.
type MappingService struct {
	categories CategoryRepository
	accounts   AccountRepository
	catalog    CategoryCatalog
	jobs       JobRepository
	settings   SettingsRepository
	waker      JobWaker
}

func NewMappingService(
	categories CategoryRepository,
	accounts AccountRepository,
	catalog CategoryCatalog,
	jobs JobRepository,
	settings SettingsRepository,
	waker JobWaker,
) *MappingService {
	return &MappingService{
		categories: categories,
		accounts:   accounts,
		catalog:    catalog,
		jobs:       jobs,
		settings:   settings,
		waker:      waker,
	}
}

func (s *MappingService) MappingMode(ctx context.Context) (domain.MappingMode, error) {
	return s.settings.MappingMode(ctx)
}

// SetMappingMode перемикає режим і будить воркер: джоби, що вже чекають,
// перевіряються за новим режимом одразу, а не на наступному тіку.
func (s *MappingService) SetMappingMode(ctx context.Context, mode domain.MappingMode) error {
	if !mode.Valid() {
		return fmt.Errorf("%w: отримано %q", domain.ErrInvalidMode, mode)
	}
	if err := s.settings.SetMappingMode(ctx, mode); err != nil {
		return err
	}
	s.waker.Wake()
	return nil
}

// WaitingMerchants — продавці, на правило для яких чекають джоби (режим merchant).
// Продавця, для якого категорія вже знаходиться (правило продавця або MCC), не
// показуємо: його джоби чекають лише рахунок, і це видно в списку рахунків.
func (s *MappingService) WaitingMerchants(ctx context.Context) ([]domain.WaitingMerchant, error) {
	merchants, err := s.jobs.WaitingMerchants(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]domain.WaitingMerchant, 0, len(merchants))
	for _, m := range merchants {
		tx := domain.BankTransaction{MCC: m.MCC, Description: m.Description, Amount: directionSign(m.Direction)}
		suggestion, err := suggestCategory(ctx, s.categories, &tx, false)
		if err != nil {
			return nil, err
		}
		if suggestion.CategoryID == "" {
			result = append(result, m)
		}
	}
	return result, nil
}

// directionSign — сума-замінник для BankTransaction, у якої відомий лише напрямок
// (Direction() визначає його за знаком суми).
func directionSign(direction domain.Direction) int64 {
	if direction == domain.DirectionExpense {
		return -1
	}
	return 1
}

// CreateMerchantRule додає правило продавця. alsoForMCC — заразом правило на весь MCC
// («для всього MCC 5818»): тоді решта продавців цього MCC теж підуть за ним.
// queued — скільки джоб цього продавця чекало (з alsoForMCC — усіх джоб MCC).
func (s *MappingService) CreateMerchantRule(ctx context.Context, rule domain.MerchantRule, alsoForMCC bool) (*domain.MerchantRule, int, error) {
	rule.Description = strings.TrimSpace(rule.Description)
	rule.Enabled = true
	if err := rule.Validate(); err != nil {
		return nil, 0, err
	}

	created, err := s.categories.AddMerchantRule(ctx, rule)
	if err != nil {
		return nil, 0, err
	}

	if alsoForMCC {
		// CreateRule сам рахує queued по всьому MCC і будить воркер
		_, queued, err := s.CreateRule(ctx, domain.MCCCategoryRule{
			MCCFrom:       rule.MCC,
			MCCTo:         rule.MCC,
			Direction:     rule.Direction,
			EzbCategoryID: rule.EzbCategoryID,
			Enabled:       true,
			Note:          "з правила продавця «" + rule.Description + "»",
		})
		if err != nil {
			// правило продавця вже збережено — кажемо, що не вдалося саме з MCC
			return created, 0, fmt.Errorf("правило для «%s» збережено, а для всього MCC %d — ні: %w", rule.Description, rule.MCC, err)
		}
		return created, queued, nil
	}

	queued := 0
	if merchants, err := s.jobs.WaitingMerchants(ctx); err == nil {
		for _, m := range merchants {
			if m.MCC == created.MCC && m.Direction == created.Direction && m.Description == created.Description {
				queued = m.WaitingJobs
			}
		}
	}
	s.waker.Wake()
	return created, queued, nil
}

func (s *MappingService) MerchantRules(ctx context.Context) ([]domain.MerchantRule, error) {
	return s.categories.ListMerchantRules(ctx)
}

// UpdateMerchantRule — інша категорія або увімкнути/вимкнути; будить воркер, як і UpdateRule.
func (s *MappingService) UpdateMerchantRule(ctx context.Context, id int64, categoryID string, enabled bool) (*domain.MerchantRule, error) {
	if categoryID == "" {
		return nil, domain.ErrEmptyCategory
	}
	updated, err := s.categories.UpdateMerchantRule(ctx, id, categoryID, enabled)
	if err != nil {
		return nil, err
	}
	s.waker.Wake()
	return updated, nil
}

// ReviewQueue — транзакції на підтвердження з пропозицією категорії. Пропозиція
// рахується щоразу заново: нове правило одразу змінює її для тих, що вже чекають.
func (s *MappingService) ReviewQueue(ctx context.Context) ([]domain.ReviewItem, error) {
	jobs, err := s.jobs.ListNeedsReview(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]domain.ReviewItem, 0, len(jobs))
	for _, job := range jobs {
		suggestion, err := suggestCategory(ctx, s.categories, &job.Tx, true)
		if err != nil {
			return nil, err
		}
		items = append(items, domain.ReviewItem{Job: job, Suggestion: suggestion})
	}
	return items, nil
}

// ConfirmJob — людина погодила категорію (запропоновану чи свою): джоба
// повертається в pending з нею, і воркер одразу відправляє її в ezBookkeeping.
func (s *MappingService) ConfirmJob(ctx context.Context, id int64, categoryID string) error {
	if categoryID == "" {
		return domain.ErrEmptyCategory
	}
	if err := s.jobs.Confirm(ctx, id, categoryID); err != nil {
		return err
	}
	s.waker.Wake()
	return nil
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

	queued := s.queuedForRule(ctx, created)
	s.waker.Wake()
	return created, queued, nil
}

// Rules — усі правила MCC → категорія, разом з вимкненими: їх теж можна редагувати.
func (s *MappingService) Rules(ctx context.Context) ([]domain.MCCCategoryRule, error) {
	return s.categories.ListRules(ctx)
}

// UpdateRule змінює категорію / enabled / нотатку правила і будить воркер:
// увімкнене правило могло розблокувати джоби. Уже записані в ezBookkeeping
// транзакції не переписуються.
func (s *MappingService) UpdateRule(ctx context.Context, id int64, categoryID string, enabled bool, note string) (*domain.MCCCategoryRule, int, error) {
	if categoryID == "" {
		return nil, 0, domain.ErrEmptyCategory
	}

	updated, err := s.categories.UpdateRule(ctx, id, categoryID, enabled, note)
	if err != nil {
		return nil, 0, err
	}

	queued := 0
	if updated.Enabled {
		queued = s.queuedForRule(ctx, updated)
	}
	s.waker.Wake()
	return updated, queued, nil
}

// queuedForRule — скільки джоб, що чекають, підпадає під правило.
// Лічильник лише для UI; правило вже збережено, тому його помилку не віддаємо.
func (s *MappingService) queuedForRule(ctx context.Context, rule *domain.MCCCategoryRule) int {
	counts, err := s.jobs.WaitingCounts(ctx)
	if err != nil {
		return 0
	}
	queued := 0
	for key, n := range counts {
		if key.Direction == rule.Direction && key.MCC >= rule.MCCFrom && key.MCC <= rule.MCCTo {
			queued += n
		}
	}
	return queued
}

// AccountMappings — усі збережені мапінги банківських рахунків.
func (s *MappingService) AccountMappings(ctx context.Context) ([]domain.AccountMapping, error) {
	return s.accounts.ListMappings(ctx)
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
	seen := make(map[domain.WaitingKey]bool, len(keys))
	for i := range keys {
		if keys[i].KeyType != domain.KeyMCC {
			continue
		}
		mcc, err := strconv.Atoi(keys[i].Key)
		if err != nil {
			continue
		}
		key := domain.WaitingKey{MCC: mcc, Direction: keys[i].Direction}
		keys[i].WaitingJobs = counts[key]
		seen[key] = true
	}

	if filter.KeyType == "" || filter.KeyType == domain.KeyMCC {
		missing, err := s.waitingWithoutHistory(ctx, seen)
		if err != nil {
			return nil, err
		}
		// нові ключі — найактуальніший борг, тому на початок
		keys = append(missing, keys...)
	}
	return keys, nil
}

// waitingWithoutHistory — MCC, на які чекають джоби, але яких немає в unmapped_keys.
// Ключ туди пишеться лише при першому переході pending → waiting у режимі mcc;
// джоба, що чекала в іншому режимі, після перемикання на mcc інакше була б невидимою.
func (s *MappingService) waitingWithoutHistory(ctx context.Context, seen map[domain.WaitingKey]bool) ([]domain.UnmappedKey, error) {
	merchants, err := s.jobs.WaitingMerchants(ctx)
	if err != nil {
		return nil, err
	}

	byKey := make(map[domain.WaitingKey]int) // індекс у result: вказівник застарів би після append
	result := make([]domain.UnmappedKey, 0)
	for _, m := range merchants {
		key := domain.WaitingKey{MCC: m.MCC, Direction: m.Direction}
		if seen[key] {
			continue
		}
		if i, ok := byKey[key]; ok {
			result[i].WaitingJobs += m.WaitingJobs
			result[i].Hits += m.WaitingJobs
			continue
		}
		// джоба може чекати лише рахунок — тоді правило є і ключ не борг
		rule, err := s.categories.EzCategoryRuleByMCC(ctx, m.MCC, m.Direction)
		if err != nil {
			return nil, err
		}
		if rule != nil {
			seen[key] = true
			continue
		}
		result = append(result, domain.UnmappedKey{
			KeyType:     domain.KeyMCC,
			Key:         strconv.Itoa(m.MCC),
			Direction:   m.Direction,
			SampleDesc:  m.Description,
			Hits:        m.WaitingJobs,
			WaitingJobs: m.WaitingJobs,
		})
		byKey[key] = len(result) - 1
	}
	return result, nil
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
