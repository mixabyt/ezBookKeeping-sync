package monobank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"ezbkTools/internal/app"
	"ezbkTools/internal/platform/logctx"
)

const setWebhookPath = "/personal/webhook"

// setWebhookTimeout більший за таймаут гарячого шляху: під час цього запиту
// банк синхронно робить GET на адресу вебхука, щоб переконатися, що вона жива.
const setWebhookTimeout = 20 * time.Second

// WebhookRegistrar реєструє адресу вебхука особистим токеном клієнта.
type WebhookRegistrar struct {
	baseURL string
	client  *http.Client
}

func NewWebhookRegistrar() *WebhookRegistrar {
	return &WebhookRegistrar{baseURL: base, client: &http.Client{Timeout: setWebhookTimeout}}
}

// SetWebhook — POST /personal/webhook. Токен іде лише в заголовку і ніде не логується.
func (r *WebhookRegistrar) SetWebhook(ctx context.Context, monoToken, webhookURL string) error {
	body, err := json.Marshal(struct {
		WebHookURL string `json:"webHookUrl"`
	}{WebHookURL: webhookURL})
	if err != nil {
		return fmt.Errorf("marshal webhook request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+setWebhookPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request %s: %w", setWebhookPath, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Token", monoToken)

	resp, err := r.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		logctx.From(ctx).Warn("Monobank не відповів на реєстрацію вебхука", "err", err)
		return app.ErrUpstreamUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
	logctx.From(ctx).Warn("Monobank відхилив реєстрацію вебхука", "status", resp.StatusCode, "body", string(raw))

	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return app.ErrBadMonoToken
	case resp.StatusCode == http.StatusTooManyRequests:
		return app.ErrMonoRateLimited
	case resp.StatusCode >= 500:
		return app.ErrUpstreamUnavailable
	default:
		return app.ErrMonoRejected
	}
}
