package app

import (
	"context"
	"errors"
	"ezbkTools/internal/domain"
	"ezbkTools/internal/platform/logctx"
	"fmt"
	"strconv"
	"strings"
)

// ErrNoCategoryRule / ErrNoAccountMapping — не збій обробки, а «чекаємо людину»:
// джоба переходить у waiting_mapping і рушить далі, щойно мапінг зʼявиться.
var (
	ErrNoCategoryRule   = errors.New("немає правила категорії")
	ErrNoAccountMapping = errors.New("немає мапінгу рахунку")
)

type SyncService struct {
	jobs          JobRepository
	categories    CategoryRepository
	accounts      AccountRepository
	ezBookKeeping Bookkeeper
	wakeUp        chan struct{}
}

func NewSyncService(
	jobs JobRepository,
	book Bookkeeper,
	categories CategoryRepository,
	accounts AccountRepository,
) *SyncService {
	return &SyncService{
		jobs:          jobs,
		wakeUp:        make(chan struct{}, 1),
		ezBookKeeping: book,
		categories:    categories,
		accounts:      accounts,
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
	s.Wake()
	return nil
}

// Wake будить воркер. Канал з буфером 1 і неблокуюча відправка: кілька сигналів
// поспіль зливаються в один — воркер однаково перебере всі джоби за прохід.
func (s *SyncService) Wake() {
	select {
	case s.wakeUp <- struct{}{}:
	default:
	}
}

func (s *SyncService) WakesUp() <-chan struct{} {
	return s.wakeUp
}

// Process — викликає воркер для pending- і waiting_mapping-джоб.
//
// Транзакція йде в ezBookkeeping лише коли відомі і категорія, і рахунок. Обидва
// перевіряються за один прохід, щоб у причині очікування одразу було все, чого бракує,
// а не "домапив категорію → тепер виявилось, що й рахунку немає".
func (s *SyncService) Process(ctx context.Context, job *domain.Job) error {
	tx := &job.Tx
	categoryID, catErr := s.categoryIDFor(ctx, tx)
	accountID, accErr := s.accountIDFor(ctx, tx)

	var missing []string
	for _, err := range []error{catErr, accErr} {
		switch {
		case err == nil:
		case errors.Is(err, ErrNoCategoryRule), errors.Is(err, ErrNoAccountMapping):
			missing = append(missing, err.Error())
		default:
			// справжній збій (БД тощо) — це не очікування людини
			logctx.From(ctx).Error("не вдалося підготувати джобу", "job", job.ID, "err", err)
			return errors.Join(s.jobs.MarkFailed(ctx, job.ID, err), err)
		}
	}

	if len(missing) > 0 {
		firstTime := job.Status == domain.JobPending
		// hits у unmapped_keys рахує транзакції, а не спроби: waiting-джоби
		// перевіряються на кожному тіку, і без цієї умови лічильник ріс би безкінечно
		if firstTime && errors.Is(catErr, ErrNoCategoryRule) {
			s.trackUnmapped(ctx, domain.UnmappedKey{
				Source:     tx.Source,
				KeyType:    domain.KeyMCC,
				Key:        strconv.Itoa(tx.MCC),
				Direction:  tx.Direction(),
				SampleDesc: tx.Description,
			})
		}

		reason := strings.Join(missing, "; ")
		if firstTime {
			logctx.From(ctx).Warn("джоба чекає мапінгу", "job", job.ID, "tx", tx.ID, "reason", reason)
		}
		return s.jobs.MarkWaiting(ctx, job.ID, reason)
	}

	err := s.ezBookKeeping.AddTransaction(ctx, tx, categoryID, accountID)
	if err != nil {
		return errors.Join(
			s.jobs.MarkFailed(ctx, job.ID, err),
			err,
		)
	}
	return s.jobs.MarkDone(ctx, job.ID)
}

// categoryIDFor — категорія за правилом MCC. Дефолту немає навмисно: транзакція
// в "Іншому" — це помилка, яку потім доводиться виправляти руками в ezBookkeeping.
func (s *SyncService) categoryIDFor(ctx context.Context, tx *domain.BankTransaction) (string, error) {
	rule, err := s.categories.EzCategoryRuleByMCC(ctx, tx.MCC)
	if err != nil {
		return "", err
	}
	if rule == nil {
		return "", fmt.Errorf("%w для MCC %d (%s)", ErrNoCategoryRule, tx.MCC, tx.Direction())
	}
	return rule.EzbCategoryID, nil
}

// accountIDFor — рахунок ezBookkeeping за мапінгом банківського рахунку.
func (s *SyncService) accountIDFor(ctx context.Context, tx *domain.BankTransaction) (string, error) {
	mapping, err := s.accounts.EzAccountByBank(ctx, tx.Source, tx.AccountID)
	if err != nil {
		return "", err
	}
	if mapping == nil {
		return "", fmt.Errorf("%w %s/%s", ErrNoAccountMapping, tx.Source, tx.AccountID)
	}
	return mapping.EzbAccountID, nil
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
