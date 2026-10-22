package sqlite

import (
	"context"
	"database/sql"
	"ezbkTools/internal/app"
	"ezbkTools/internal/domain"
	"fmt"
	"strings"
	"time"
)

type Job struct {
	db *sql.DB
}

func NewJob(db *sql.DB) *Job {
	return &Job{db}
}

func (j *Job) Add(ctx context.Context, tx *domain.BankTransaction) error {
	query := `INSERT INTO jobs (tx_id, account_id, tx_time, amount, mcc, description, hold, source)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := j.db.ExecContext(ctx, query,
		tx.ID,
		tx.AccountID,
		tx.Time.Unix(),
		tx.Amount,
		tx.MCC,
		tx.Description,
		tx.Hold,
		tx.Source,
	)
	if err != nil {
		return fmt.Errorf("insert job for tx %s: %w", tx.ID, err)
	}
	return nil
}

func (j *Job) FetchPending(ctx context.Context, limit int) ([]domain.Job, error) {
	return j.fetchByStatus(ctx, 0, limit, domain.JobPending)
}

// FetchWaiting — джоби, яким бракувало мапінгу або підтвердження. Воркер перебирає їх
// на кожному тіку: людина могла домапити категорію й рахунок або змінити режим.
func (j *Job) FetchWaiting(ctx context.Context, afterID int64, limit int) ([]domain.Job, error) {
	return j.fetchByStatus(ctx, afterID, limit, domain.JobWaitingMapping, domain.JobNeedsReview)
}

// ListNeedsReview — усі джоби на підтвердження, найстаріші першими.
func (j *Job) ListNeedsReview(ctx context.Context) ([]domain.Job, error) {
	return j.fetchByStatus(ctx, 0, -1, domain.JobNeedsReview) // LIMIT -1 у SQLite — без обмеження
}

func (j *Job) fetchByStatus(ctx context.Context, afterID int64, limit int, statuses ...domain.JobStatus) ([]domain.Job, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(statuses)), ",")
	query := `SELECT id, tx_id, account_id, tx_time, amount, mcc, description, hold,
			         status, attempts, last_error, ezb_category_id, source
			  FROM jobs
			  WHERE status IN (` + placeholders + `) AND id > ?
			  ORDER BY id
			  LIMIT ?`

	args := make([]any, 0, len(statuses)+2)
	for _, status := range statuses {
		args = append(args, status)
	}
	args = append(args, afterID, limit)

	rows, err := j.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch %v jobs: %w", statuses, err)
	}
	defer rows.Close()

	jobs := make([]domain.Job, 0, max(limit, 0))
	for rows.Next() {
		var (
			job    domain.Job
			txTime int64
		)
		if err := rows.Scan(
			&job.ID,
			&job.Tx.ID,
			&job.Tx.AccountID,
			&txTime,
			&job.Tx.Amount,
			&job.Tx.MCC,
			&job.Tx.Description,
			&job.Tx.Hold,
			&job.Status,
			&job.Attempts,
			&job.LastError,
			&job.CategoryID,
			&job.Tx.Source,
		); err != nil {
			return nil, fmt.Errorf("scan %v job: %w", statuses, err)
		}
		job.Tx.Time = time.Unix(txTime, 0)
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %v jobs: %w", statuses, err)
	}

	return jobs, nil
}

// MarkDone очищає last_error: джоба могла пройти через waiting_mapping,
// і стара причина очікування на успішній джобі виглядала б як помилка.
// categoryID — з якою категорією джоба пішла (джерело пропозиції «як минулого разу»).
func (j *Job) MarkDone(ctx context.Context, id int64, categoryID string) error {
	query := `UPDATE jobs SET status = ?, last_error = '', ezb_category_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := j.db.ExecContext(ctx, query, domain.JobDone, categoryID, id)
	if err != nil {
		return fmt.Errorf("mark done job %d: %w", id, err)
	}
	return nil
}

func (j *Job) MarkFailed(ctx context.Context, id int64, err error) error {
	query := `UPDATE jobs SET status = ? WHERE id = ?`
	_, err = j.db.ExecContext(ctx, query, domain.JobFailed, id)
	if err != nil {
		return fmt.Errorf("mark failed job %d: %w", id, err)
	}
	return nil
}

// MarkWaiting — джобі бракує мапінгу. reason кладемо в last_error, щоб з БД
// було видно, чого саме чекає джоба, без пошуку по логах.
func (j *Job) MarkWaiting(ctx context.Context, id int64, reason string) error {
	query := `UPDATE jobs SET status = ?, last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := j.db.ExecContext(ctx, query, domain.JobWaitingMapping, reason, id)
	if err != nil {
		return fmt.Errorf("mark waiting job %d: %w", id, err)
	}
	return nil
}

// WaitingCounts — скільки джоб чекає на кожну пару (MCC, напрямок).
// Напрямок у jobs не зберігається: як і BankTransaction.Direction(), рахуємо його зі знаку суми.
func (j *Job) WaitingCounts(ctx context.Context) (map[domain.WaitingKey]int, error) {
	query := `SELECT mcc, amount < 0 AS is_expense, count(*)
			  FROM jobs
			  WHERE status = ?
			  GROUP BY mcc, is_expense`
	rows, err := j.db.QueryContext(ctx, query, domain.JobWaitingMapping)
	if err != nil {
		return nil, fmt.Errorf("waiting counts: %w", err)
	}
	defer rows.Close()

	counts := make(map[domain.WaitingKey]int)
	for rows.Next() {
		var (
			mcc, count int
			isExpense  bool
		)
		if err := rows.Scan(&mcc, &isExpense, &count); err != nil {
			return nil, fmt.Errorf("scan waiting count: %w", err)
		}
		direction := domain.DirectionIncome
		if isExpense {
			direction = domain.DirectionExpense
		}
		counts[domain.WaitingKey{MCC: mcc, Direction: direction}] = count
	}
	return counts, rows.Err()
}

// WaitingAccounts — банківські рахунки, з яких є джоби в очікуванні.
// Чи є для них мапінг, вирішує app: тут лише факт "з цього рахунку щось чекає".
func (j *Job) WaitingAccounts(ctx context.Context) ([]domain.WaitingAccount, error) {
	query := `SELECT source, account_id, count(*), max(description)
			  FROM jobs
			  WHERE status = ?
			  GROUP BY source, account_id
			  ORDER BY count(*) DESC`
	rows, err := j.db.QueryContext(ctx, query, domain.JobWaitingMapping)
	if err != nil {
		return nil, fmt.Errorf("waiting accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]domain.WaitingAccount, 0)
	for rows.Next() {
		var account domain.WaitingAccount
		if err := rows.Scan(&account.Source, &account.BankAccountID, &account.WaitingJobs, &account.SampleDesc); err != nil {
			return nil, fmt.Errorf("scan waiting account: %w", err)
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

// MarkNeedsReview — режим manual: категорію має підтвердити людина.
func (j *Job) MarkNeedsReview(ctx context.Context, id int64) error {
	query := `UPDATE jobs SET status = ?, last_error = '', updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := j.db.ExecContext(ctx, query, domain.JobNeedsReview, id)
	if err != nil {
		return fmt.Errorf("mark needs review job %d: %w", id, err)
	}
	return nil
}

// Confirm — needs_review → pending з категорією, яку обрала людина. Умова на статус
// у самому UPDATE: подвійний тап чи друга вкладка не відправлять транзакцію двічі.
func (j *Job) Confirm(ctx context.Context, id int64, categoryID string) error {
	query := `UPDATE jobs SET status = ?, ezb_category_id = ?, updated_at = CURRENT_TIMESTAMP
			  WHERE id = ? AND status = ?`
	res, err := j.db.ExecContext(ctx, query, domain.JobPending, categoryID, id, domain.JobNeedsReview)
	if err != nil {
		return fmt.Errorf("confirm job %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: id %d", app.ErrJobNotReviewable, id)
	}
	return nil
}

// WaitingMerchants — продавці (MCC + напрямок + опис), через яких стоять джоби.
// trim у SQL — той самий ключ, що й merchantDescription в app.
func (j *Job) WaitingMerchants(ctx context.Context) ([]domain.WaitingMerchant, error) {
	query := `SELECT mcc, amount < 0 AS is_expense, trim(description) AS merchant, count(*)
			  FROM jobs
			  WHERE status = ?
			  GROUP BY mcc, is_expense, merchant
			  ORDER BY count(*) DESC, max(id) DESC`
	rows, err := j.db.QueryContext(ctx, query, domain.JobWaitingMapping)
	if err != nil {
		return nil, fmt.Errorf("waiting merchants: %w", err)
	}
	defer rows.Close()

	merchants := make([]domain.WaitingMerchant, 0)
	for rows.Next() {
		var (
			m         domain.WaitingMerchant
			isExpense bool
		)
		if err := rows.Scan(&m.MCC, &isExpense, &m.Description, &m.WaitingJobs); err != nil {
			return nil, fmt.Errorf("scan waiting merchant: %w", err)
		}
		m.Direction = domain.DirectionIncome
		if isExpense {
			m.Direction = domain.DirectionExpense
		}
		merchants = append(merchants, m)
	}
	return merchants, rows.Err()
}
