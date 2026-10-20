package domain

type JobStatus string

const (
	JobPending JobStatus = "pending"
	JobDone    JobStatus = "done"
	JobFailed  JobStatus = "failed"
)

type Job struct {
	ID        int64
	Tx        BankTransaction
	Status    JobStatus
	Attempts  int
	LastError string
}
