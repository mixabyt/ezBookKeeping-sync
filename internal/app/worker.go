package app

import (
	"context"
	"ezbkTools/internal/domain"
	"ezbkTools/internal/platform/logctx"
	"time"
)

var (
	FetchLimit = 50
	Delay      = 15 * time.Minute
)

type Worker struct {
	jobs JobRepository
	svc  *SyncService
}

func NewWorker(jobs JobRepository, svc *SyncService) *Worker {
	return &Worker{
		jobs: jobs,
		svc:  svc,
	}
}

func (w *Worker) Run(ctx context.Context) {
	logctx.From(ctx).Info("Воркер запущено")
	ticker := time.NewTicker(Delay)
	for {
		w.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-w.svc.WakesUp():
			logctx.From(ctx).Info("отримано сигнал")
		case <-ticker.C:
		}
	}
}

func (w *Worker) drain(ctx context.Context) {
	for ctx.Err() == nil {
		jobs, err := w.jobs.FetchPending(ctx, FetchLimit)
		if err != nil {
			logctx.From(ctx).Error("fetch pending jobs: ", err)
		}
		for _, job := range jobs {
			if ctx.Err() != nil {
				return
			}
			w.handle(ctx, &job)
		}
		if len(jobs) < FetchLimit {
			return
		}
	}
}

func (w *Worker) handle(ctx context.Context, job *domain.Job) {
	err := w.svc.Process(ctx, job)
	if err != nil {
		logctx.From(ctx).Error("process job: ", job.ID, err)
	}

}
