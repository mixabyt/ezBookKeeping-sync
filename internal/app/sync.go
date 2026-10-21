package app

import (
	"context"
	"errors"
	"ezbkTools/internal/domain"
	"ezbkTools/internal/platform/logctx"
	"fmt"
	"strconv"
)

type SyncService struct {
	jobs          JobRepository
	categories    CategoryRepository
	ezBookKeeping Bookkeeper
	defaults      domain.Defaults
	wakeUp        chan struct{}
}

// categories CategoryRepository, ezBookKeeping Bookkeeper
// categories: categories, ezBookKeeping: ezBookKeeping
func NewSyncService(
	jobs JobRepository,
	book Bookkeeper,
	categories CategoryRepository,
	defaults domain.Defaults,
) *SyncService {
	return &SyncService{
		jobs:          jobs,
		wakeUp:        make(chan struct{}, 1),
		ezBookKeeping: book,
		categories:    categories,
		defaults:      defaults,
	}
}

func (s *SyncService) Enqueue(ctx context.Context, tx *domain.BankTransaction) error {
	if tx.MCC == 0 || tx.BankCategory != "" {
		mcc, err := s.normalizeTxCategory(ctx, tx)
		if err != nil {
			logctx.From(ctx).Error("normalizeTxCategory", "tx", tx.ID, "err", err)
			return err
		}
		tx.MCC = mcc
	}
	err := s.jobs.Add(ctx, tx)
	if err != nil {
		logctx.From(ctx).Error("Помилка створення job", "tx", tx.ID, "err", err)
		return err
	}
	logctx.From(ctx).Info("Створено job", "tx", tx.ID)
	select {
	case s.wakeUp <- struct{}{}:
		logctx.From(ctx).Info("Надіслано сигнал")
	default:
	}
	return nil
}

func (s *SyncService) WakesUp() <-chan struct{} {
	return s.wakeUp
}

// Process — викликає воркер для кожної pending-джоби.
func (s *SyncService) Process(ctx context.Context, job *domain.Job) error {
	categoryID, err := s.categoryIDFor(ctx, &job.Tx)
	if err != nil {
		logctx.From(ctx).Error("не вдалося визначити категорію", "job", job.ID, "err", err)
		return errors.Join(
			s.jobs.MarkFailed(ctx, job.ID, err),
			err,
		)
	}

	accountID, err := s.accountIDFor(ctx, &job.Tx)
	if err != nil {
		logctx.From(ctx).Error("не вдалося визначити рахунок", "job", job.ID, "err", err)
		return errors.Join(
			s.jobs.MarkFailed(ctx, job.ID, err),
			err,
		)
	}

	err = s.ezBookKeeping.AddTransaction(ctx, &job.Tx, categoryID, accountID)
	if err != nil {
		return errors.Join(
			s.jobs.MarkFailed(ctx, job.ID, err),
			err,
		)
	}
	return s.jobs.MarkDone(ctx, job.ID)
}

// categoryIDFor — категорія за правилом MCC, а якщо правила немає — дефолт за напрямком.
// Без дефолту транзакцію не записуємо: краще failed-джоба, ніж запис у випадкову категорію.
func (s *SyncService) categoryIDFor(ctx context.Context, tx *domain.BankTransaction) (string, error) {
	rule, err := s.categories.EzCategoryRuleByMCC(ctx, tx.MCC)
	if err != nil {
		return "", err
	}
	if rule != nil {
		return rule.EzbCategoryID, nil
	}

	categoryID := s.defaults.CategoryFor(tx.Direction())
	if categoryID == "" {
		return "", fmt.Errorf(
			"немає правила для MCC %d і не налаштована дефолтна категорія для %s",
			tx.MCC, tx.Direction(),
		)
	}

	s.trackUnmapped(ctx, domain.UnmappedKey{
		Source:     tx.Source,
		KeyType:    domain.KeyMCC,
		Key:        strconv.Itoa(tx.MCC),
		Direction:  tx.Direction(),
		SampleDesc: tx.Description,
	})

	logctx.From(ctx).Warn(
		"правила для MCC немає, беремо дефолтну категорію",
		"mcc", tx.MCC, "direction", tx.Direction(), "category_id", categoryID,
	)
	return categoryID, nil
}

// accountIDFor — рахунок ezBookkeeping для банківського рахунку транзакції.
// TODO: мапінг account_mappings (source + bank_account_id → ezb_account_id);
// поки один рахунок на все — дефолт з конфігу.
func (s *SyncService) accountIDFor(_ context.Context, tx *domain.BankTransaction) (string, error) {
	if s.defaults.SourceAccountID == "" {
		return "", fmt.Errorf("не налаштований дефолтний рахунок для %s/%s", tx.Source, tx.AccountID)
	}
	return s.defaults.SourceAccountID, nil
}

// trackUnmapped — запис ключа без мапінгу. Це підказка на майбутнє, а не частина
// обробки транзакції, тому помилка лише логується: через неї втрачати транзакцію безглуздо.
func (s *SyncService) trackUnmapped(ctx context.Context, key domain.UnmappedKey) {
	if err := s.categories.TrackUnmapped(ctx, key); err != nil {
		logctx.From(ctx).Error("не вдалося записати unmapped key",
			"key_type", key.KeyType, "key", key.Key, "err", err)
	}
}

// помилка може виникнути лише коли проблеми з бд
// int 0 - категорія не знайдена
func (s *SyncService) normalizeTxCategory(ctx context.Context, tx *domain.BankTransaction) (int, error) {
	bankCat, err := s.categories.MccByBankCategory(ctx, tx.Source, tx.BankCategory)
	if err != nil {
		return 0, err
	}
	if bankCat != nil {
		return bankCat.MCC, nil
	}

	s.trackUnmapped(ctx, domain.UnmappedKey{
		Source:     tx.Source,
		KeyType:    domain.KeyBankCategory,
		Key:        tx.BankCategory,
		Direction:  tx.Direction(),
		SampleDesc: tx.Description,
	})
	return 0, nil
}
