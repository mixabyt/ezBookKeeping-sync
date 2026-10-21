package sqlite

import (
	"context"
	"database/sql"
	"ezbkTools/internal/domain"
	"fmt"
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
	return j.fetchByStatus(ctx, domain.JobPending, 0, limit)
}

// FetchWaiting — джоби, яким бракувало мапінгу. Воркер перебирає їх на кожному тіку:
// якщо людина вже домапила категорію й рахунок, джоба піде далі.
func (j *Job) FetchWaiting(ctx context.Context, afterID int64, limit int) ([]domain.Job, error) {
	return j.fetchByStatus(ctx, domain.JobWaitingMapping, afterID, limit)
}

func (j *Job) fetchByStatus(ctx context.Context, status domain.JobStatus, afterID int64, limit int) ([]domain.Job, error) {
	query := `SELECT id, tx_id, account_id, tx_time, amount, mcc, description, hold,
			         status, attempts, last_error, source
			  FROM jobs
			  WHERE status = ? AND id > ?
			  ORDER BY id
			  LIMIT ?`
	rows, err := j.db.QueryContext(ctx, query, status, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("fetch %s jobs: %w", status, err)
	}
	defer rows.Close()

	jobs := make([]domain.Job, 0, limit)
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
			&job.Tx.Source,
		); err != nil {
			return nil, fmt.Errorf("scan %s job: %w", status, err)
		}
		job.Tx.Time = time.Unix(txTime, 0)
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s jobs: %w", status, err)
	}

	return jobs, nil
}

// MarkDone очищає last_error: джоба могла пройти через waiting_mapping,
// і стара причина очікування на успішній джобі виглядала б як помилка.
func (j *Job) MarkDone(ctx context.Context, id int64) error {
	query := `UPDATE jobs SET status = ?, last_error = '', updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := j.db.ExecContext(ctx, query, domain.JobDone, id)
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
