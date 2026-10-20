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
	query := `SELECT id, tx_id, account_id, tx_time, amount, mcc, description, hold,
			         status, attempts, last_error, source
			  FROM jobs
			  WHERE status = ?
			  ORDER BY id
			  LIMIT ?`
	rows, err := j.db.QueryContext(ctx, query, domain.JobPending, limit)
	if err != nil {
		return nil, fmt.Errorf("fetch pending jobs: %w", err)
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
			return nil, fmt.Errorf("scan pending job: %w", err)
		}
		job.Tx.Time = time.Unix(txTime, 0)
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending jobs: %w", err)
	}

	return jobs, nil
}

func (j *Job) MarkDone(ctx context.Context, id int64) error {
	query := `UPDATE jobs SET status = ? WHERE id = ?`
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
