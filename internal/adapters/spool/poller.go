package spool

import (
	"context"
	"errors"
	"time"

	"ezbkTools/internal/platform/logctx"
)

// ErrDiscard — запит ніколи не вдасться обробити (зламаний файл, підробка, не той ендпоінт).
// Такий файл видаляється одразу, а не лежить у черзі вічно.
var ErrDiscard = errors.New("spool: discard request")

// Handler обробляє один збережений запит.
// nil — оброблено, файл видаляється.
// Помилка з ErrDiscard — файл видаляється без обробки.
// Будь-яка інша помилка — файл лишається до наступного проходу.
type Handler func(ctx context.Context, req Request) error

// Poller забирає запити з черги на VPS: один раз при старті і далі з інтервалом.
type Poller struct {
	client   *Client
	handle   Handler
	interval time.Duration
}

func NewPoller(client *Client, handle Handler, interval time.Duration) *Poller {
	return &Poller{client: client, handle: handle, interval: interval}
}

// Drain обробляє всі файли, які зараз лежать у черзі.
// Повертає помилку лише якщо не вдалося отримати список; збої окремих файлів тільки логуються.
func (p *Poller) Drain(ctx context.Context) error {
	names, err := p.client.List(ctx)
	if err != nil {
		return err
	}

	for _, name := range names {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.processFile(ctx, name)
	}
	return nil
}

// Run періодично викликає Drain, поки ctx не скасовано. Перший прохід — через interval:
// стартовий робить main, щоб він завершився до запуску HTTP-сервера.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.Drain(ctx); err != nil && ctx.Err() == nil {
				logctx.From(ctx).Warn("spool drain failed", "err", err)
			}
		}
	}
}

func (p *Poller) processFile(ctx context.Context, name string) {
	log := logctx.From(ctx).With("spool_file", name)

	req, err := p.client.Read(ctx, name)
	if err == nil {
		err = p.handle(ctx, req)
	}

	switch {
	case err == nil:
		log.Info("spooled request processed", "upstream_status", req.UpstreamStatus)
	case errors.Is(err, ErrDiscard):
		log.Warn("spooled request discarded", "err", err)
	default:
		log.Warn("spooled request kept for retry", "err", err)
		return
	}

	if err := p.client.Delete(ctx, name); err != nil {
		log.Warn("delete spooled request failed", "err", err)
	}
}
