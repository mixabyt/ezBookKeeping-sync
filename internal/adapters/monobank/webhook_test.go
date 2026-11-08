package monobank

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ezbkTools/internal/app"
)

func TestSetWebhook(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   error
	}{
		{"успіх", http.StatusOK, nil},
		{"невірний ключ 403", http.StatusForbidden, app.ErrBadMonoToken},
		{"невірний ключ 401", http.StatusUnauthorized, app.ErrBadMonoToken},
		{"ліміт запитів", http.StatusTooManyRequests, app.ErrMonoRateLimited},
		{"банк не підтвердив адресу", http.StatusBadRequest, app.ErrMonoRejected},
		{"збій банку", http.StatusBadGateway, app.ErrUpstreamUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotToken, gotURL, gotPath string
			bank := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotToken = r.Header.Get("X-Token")
				var body struct {
					WebHookURL string `json:"webHookUrl"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				gotURL = body.WebHookURL
				w.WriteHeader(tc.status)
			}))
			defer bank.Close()

			registrar := &WebhookRegistrar{baseURL: bank.URL, client: bank.Client()}
			err := registrar.SetWebhook(context.Background(), "mono-key", "https://example.test/mono-webhook/abc")

			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, очікували %v", err, tc.want)
			}
			if gotPath != setWebhookPath || gotToken != "mono-key" || gotURL != "https://example.test/mono-webhook/abc" {
				t.Fatalf("запит: path=%q token=%q url=%q", gotPath, gotToken, gotURL)
			}
		})
	}
}

func TestSetWebhookBankUnreachable(t *testing.T) {
	bank := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := bank.URL
	bank.Close()

	registrar := &WebhookRegistrar{baseURL: url, client: http.DefaultClient}
	if err := registrar.SetWebhook(context.Background(), "k", "u"); !errors.Is(err, app.ErrUpstreamUnavailable) {
		t.Fatalf("err = %v", err)
	}
}
