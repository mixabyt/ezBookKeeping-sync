package app

import (
	"context"
	"errors"
	"ezbkTools/internal/domain"
	"ezbkTools/internal/platform/logctx"
)

type SyncService struct {
	jobs          JobRepository
	categories    CategoryRepository
	ezBookKeeping Bookkeeper
	wakeUp        chan struct{}
}

// categories CategoryRepository, ezBookKeeping Bookkeeper
// categories: categories, ezBookKeeping: ezBookKeeping
func NewSyncService(jobs JobRepository, book Bookkeeper) *SyncService {
	return &SyncService{
		jobs:          jobs,
		wakeUp:        make(chan struct{}),
		ezBookKeeping: book,
	}
}

func (s *SyncService) Enqueue(ctx context.Context, tx *domain.BankTransaction) error {
	if tx.MCC == 0 || tx.BankCategory != "" {
		mcc, err := s.normalizeTxCategory(ctx, tx)
		if err != nil {
			logctx.From(ctx).Error("normalizeTxCategory", err)
			return err
		}
		tx.MCC = mcc
	}
	err := s.jobs.Add(ctx, tx)
	if err != nil {
		logctx.From(ctx).Error("Помилка створення job", tx.ID, err)
		return err
	}
	logctx.From(ctx).Info("Створено job", tx.ID)
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
	mccCatRule, err := s.categories.EzCategoryRuleByMCC(ctx, job.Tx.MCC)
	if err != nil {
		logctx.From(ctx).Error("err with get ezcategory rule", err)
		return err
	}

	err = s.ezBookKeeping.AddTransaction(ctx, &job.Tx, mccCatRule.EzbCategoryID)
	if err != nil {
		return errors.Join(
			s.jobs.MarkFailed(ctx, job.ID, err),
			err,
		)
	}
	return s.jobs.MarkDone(ctx, job.ID)
}

// помилка може виникнути лише коли проблеми з бд
// int 0 - категорія не знайдена
func (s *SyncService) normalizeTxCategory(ctx context.Context, tx *domain.BankTransaction) (int, error) {
	bankCat, err := s.categories.MccByBankCategory(tx.Source, tx.BankCategory)
	if err != nil {
		return 0, err
	}
	if bankCat != nil {
		return bankCat.MCC, nil
	}
	return 0, nil
}
