package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ezbkTools/internal/adapters/sqlite"
	"ezbkTools/internal/adapters/sqlite/sqlitetest"
	"ezbkTools/internal/app"
	"ezbkTools/internal/domain"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const testSecret = "test-secret"

type stubLedger struct{}

func (stubLedger) AddTransaction(context.Context, *domain.BankTransaction, string, string) error {
	return nil
}
func (stubLedger) Categories(context.Context) ([]domain.EzCategory, error) { return nil, nil }
func (stubLedger) Accounts(context.Context) ([]domain.EzAccount, error)    { return nil, nil }

type stubRegistrar struct{}

func (stubRegistrar) SetWebhook(context.Context, string, string) error { return nil }

type testApp struct {
	server   *Server
	second   *domain.User
	password string
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := sqlitetest.Open(t)
	ctx := context.Background()
	users := sqlite.NewUser(db)
	jobs := sqlite.NewJob(db)
	categories := sqlite.NewCategory(db)
	accounts := sqlite.NewAccount(db)
	settings := sqlite.NewSettings(db)
	ledgers := func(string) app.Ledger { return stubLedger{} }

	hash, err := bcrypt.GenerateFromPassword([]byte("owner-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.BootstrapOwner(ctx, users, "owner", string(hash), "owner-token"); err != nil {
		t.Fatal(err)
	}
	second, password, err := app.CreateUser(ctx, users, "second")
	if err != nil {
		t.Fatal(err)
	}

	syncService := app.NewSyncService(jobs, users, ledgers, categories, accounts, settings)
	mapping := app.NewMappingService(categories, accounts, users, ledgers, jobs, settings, syncService)
	userService := app.NewUserService(users, ledgers, stubRegistrar{}, syncService, "https://example.test/mono-webhook")

	return &testApp{
		server:   NewServer(syncService, mapping, userService, AuthConfig{Secret: testSecret}),
		second:   second,
		password: password,
	}
}

func (a *testApp) do(method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	a.server.ServeHTTP(rec, req)
	return rec
}

func (a *testApp) login(t *testing.T, login, password string) *http.Cookie {
	t.Helper()
	rec := a.do(http.MethodPost, "/api/login", `{"login":"`+login+`","password":"`+password+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", login, rec.Code, rec.Body.String())
	}
	return sessionFrom(t, rec)
}

func sessionFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == sessionCookie {
			return cookie
		}
	}
	t.Fatal("немає кукі сесії")
	return nil
}

func TestSessionTokenRoundTrip(t *testing.T) {
	token, _ := issueToken(7, 3, testSecret, time.Hour)
	userID, version, err := parseToken(token, testSecret)
	if err != nil || userID != 7 || version != 3 {
		t.Fatalf("userID=%d version=%d err=%v", userID, version, err)
	}

	if _, _, err := parseToken(token, "other-secret"); err == nil {
		t.Fatal("токен з чужим секретом прийнято")
	}
	if _, _, err := parseToken(token+"x", testSecret); err == nil {
		t.Fatal("токен зі зміненим підписом прийнято")
	}

	expired, _ := issueToken(7, 3, testSecret, -time.Minute)
	if _, _, err := parseToken(expired, testSecret); err == nil {
		t.Fatal("прострочений токен прийнято")
	}
}

func TestOldFormatSessionTokenIsRejected(t *testing.T) {
	// base64url("owner|<далеке майбутнє>"), підписаний правильним секретом
	encoded := "b3duZXJ8NDEwMjQ0NDgwMA"
	if _, _, err := parseToken(encoded+"."+sign(encoded, testSecret), testSecret); err == nil {
		t.Fatal("токен старого формату прийнято")
	}
}

func TestParseMonoWebhookURI(t *testing.T) {
	cases := []struct {
		uri, token string
		ok         bool
	}{
		{"/mono-webhook", "", true},
		{"/mono-webhook?x=1", "", true},
		{"/mono-webhook/abc", "abc", true},
		{"/mono-webhook/abc?x=1", "abc", true},
		{"/mono-webhook/", "", false},
		{"/mono-webhook/abc/def", "", false},
		{"/mono-webhook-other", "", false},
		{"/api/login", "", false},
	}
	for _, tc := range cases {
		token, ok := ParseMonoWebhookURI(tc.uri)
		if token != tc.token || ok != tc.ok {
			t.Errorf("%s: token=%q ok=%v, очікували %q %v", tc.uri, token, ok, tc.token, tc.ok)
		}
	}
}

func TestTemporaryPasswordBlocksAPIUntilChanged(t *testing.T) {
	a := newTestApp(t)
	session := a.login(t, "second", a.password)

	rec := a.do(http.MethodGet, "/api/me", "", session)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"must_change_password":true`) {
		t.Fatalf("/api/me: %d %s", rec.Code, rec.Body.String())
	}

	rec = a.do(http.MethodGet, "/api/rules", "", session)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), codePasswordChange) {
		t.Fatalf("API мало бути закрите: %d %s", rec.Code, rec.Body.String())
	}

	body, _ := json.Marshal(ChangePasswordRequest{CurrentPassword: a.password, NewPassword: "brand-new-password"})
	rec = a.do(http.MethodPost, "/api/change-password", string(body), session)
	if rec.Code != http.StatusOK {
		t.Fatalf("change-password: %d %s", rec.Code, rec.Body.String())
	}
	fresh := sessionFrom(t, rec)

	if rec := a.do(http.MethodGet, "/api/rules", "", fresh); rec.Code != http.StatusOK {
		t.Fatalf("після зміни пароля API закрите: %d %s", rec.Code, rec.Body.String())
	}
	if rec := a.do(http.MethodGet, "/api/me", "", session); rec.Code != http.StatusUnauthorized {
		t.Fatalf("стара сесія мала згаснути: %d", rec.Code)
	}
}

func TestWrongCurrentPasswordIsNotSessionExpiry(t *testing.T) {
	a := newTestApp(t)
	session := a.login(t, "owner", "owner-password")

	body, _ := json.Marshal(ChangePasswordRequest{CurrentPassword: "wrong", NewPassword: "brand-new-password"})
	rec := a.do(http.MethodPost, "/api/change-password", string(body), session)
	// 401 PWA сприйняла б як завершену сесію і викинула людину на логін
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("очікували 400: %d %s", rec.Code, rec.Body.String())
	}
}

func TestMeDoesNotExposeSecrets(t *testing.T) {
	a := newTestApp(t)
	session := a.login(t, "owner", "owner-password")

	rec := a.do(http.MethodGet, "/api/me", "", session)
	if strings.Contains(rec.Body.String(), "owner-token") || strings.Contains(rec.Body.String(), "webhook_token") {
		t.Fatalf("/api/me віддає секрети: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ezb_configured":true`) {
		t.Fatalf("немає ознаки токена: %s", rec.Body.String())
	}
}

func TestWebhookRoutes(t *testing.T) {
	a := newTestApp(t)

	if rec := a.do(http.MethodGet, "/mono-webhook/"+a.second.WebhookToken, ""); rec.Code != http.StatusOK {
		t.Fatalf("перевірка адреси банком: %d", rec.Code)
	}
	if rec := a.do(http.MethodGet, "/mono-webhook/unknown", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("невідомий токен (GET): %d", rec.Code)
	}
	if rec := a.do(http.MethodPost, "/mono-webhook/unknown", "{}"); rec.Code != http.StatusNotFound {
		t.Fatalf("невідомий токен (POST): %d", rec.Code)
	}
	if rec := a.do(http.MethodGet, "/mono-webhook", ""); rec.Code != http.StatusOK {
		t.Fatalf("стара адреса: %d", rec.Code)
	}
}

func TestReplayRejectsUnknownToken(t *testing.T) {
	a := newTestApp(t)
	err := a.server.ReplayMonoWebhook(context.Background(), "unknown", http.Header{}, []byte("{}"))
	if err == nil || !strings.Contains(err.Error(), ErrInvalidWebhook.Error()) {
		t.Fatalf("очікували ErrInvalidWebhook: %v", err)
	}
}
