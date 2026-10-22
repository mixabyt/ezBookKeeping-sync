package domain

type JobStatus string

const (
	JobPending JobStatus = "pending"
	// JobWaitingMapping — для транзакції бракує категорії або рахунку; чекає,
	// поки людина домапить їх у PWA. Воркер перевіряє такі джоби на кожному тіку.
	JobWaitingMapping JobStatus = "waiting_mapping"
	// JobNeedsReview — режим manual: категорію має підтвердити людина. Після
	// підтвердження джоба повертається в pending уже з обраною категорією.
	JobNeedsReview JobStatus = "needs_review"
	JobDone        JobStatus = "done"
	JobFailed      JobStatus = "failed"
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
	// CategoryID — категорія, підтверджена людиною (manual) або з якою джоба
	// пішла в ezBookkeeping. Порожня — категорію визначать правила.
	CategoryID string
}

// WaitingMerchant — продавець (MCC + напрямок + опис), на правило для якого чекають джоби.
type WaitingMerchant struct {
	MCC         int
	Direction   Direction
	Description string
	WaitingJobs int
}

// ReviewItem — транзакція на підтвердження разом із запропонованою категорією.
type ReviewItem struct {
	Job        Job
	Suggestion Suggestion
}
