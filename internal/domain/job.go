package domain

type JobStatus string

const (
	JobPending JobStatus = "pending"
	// JobWaitingMapping — для транзакції бракує категорії або рахунку; чекає,
	// поки людина домапить їх у PWA. Воркер перевіряє такі джоби на кожному тіку.
	JobWaitingMapping JobStatus = "waiting_mapping"
	JobDone           JobStatus = "done"
	JobFailed         JobStatus = "failed"
)

// WaitingKey — за чим групуються джоби, що чекають правила категорії.
// Напрямок у jobs не зберігається, він похідний від знаку суми.
type WaitingKey struct {
	MCC       int
	Direction Direction
}

type Job struct {
	ID        int64
	Tx        BankTransaction
	Status    JobStatus
	Attempts  int
	LastError string
}
