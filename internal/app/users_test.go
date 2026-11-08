package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ezbkTools/internal/adapters/sqlite"
	"ezbkTools/internal/adapters/sqlite/sqlitetest"
	"ezbkTools/internal/app"
	"ezbkTools/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

type fakeLedger struct {
	token  string
	sent   *[]string // токени, якими записували транзакції
	reject bool
}

func (l fakeLedger) AddTransaction(context.Context, *domain.BankTransaction, string, string) error {
	*l.sent = append(*l.sent, l.token)
	return nil
}

func (l fakeLedger) Categories(context.Context) ([]domain.EzCategory, error) { return nil, nil }

func (l fakeLedger) Accounts(context.Context) ([]domain.EzAccount, error) {
	if l.reject {
		return nil, app.ErrBadEzbToken
	}
	return nil, nil
}

type fakeRegistrar struct {
	token, url string
	err        error
}

func (r *fakeRegistrar) SetWebhook(_ context.Context, token, url string) error {
	r.token, r.url = token, url
	return r.err
}

type waker struct{ woken int }

func (w *waker) Wake() { w.woken++ }

type fixture struct {
	sync      *app.SyncService
	users     *app.UserService
	userRepo  *sqlite.User
	jobs      *sqlite.Job
	accounts  *sqlite.Account
	rules     *sqlite.Category
	registrar *fakeRegistrar
	waker     *waker
	sent      *[]string
	second    *domain.User
	password  string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := sqlitetest.Open(t)
	ctx := context.Background()

	f := &fixture{
		userRepo:  sqlite.NewUser(db),
		jobs:      sqlite.NewJob(db),
		accounts:  sqlite.NewAccount(db),
		rules:     sqlite.NewCategory(db),
		registrar: &fakeRegistrar{},
		waker:     &waker{},
		sent:      &[]string{},
	}
	ledgers := func(token string) app.Ledger {
		return fakeLedger{token: token, sent: f.sent, reject: token == "bad"}
	}
	f.sync = app.NewSyncService(f.jobs, f.userRepo, ledgers, f.rules, f.accounts, sqlite.NewSettings(db))
	f.users = app.NewUserService(f.userRepo, ledgers, f.registrar, f.waker, "https://example.test/mono-webhook/")

	hash, err := bcrypt.GenerateFromPassword([]byte("owner-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.BootstrapOwner(ctx, f.userRepo, "owner", string(hash), "owner-token"); err != nil {
		t.Fatal(err)
	}
	f.second, f.password, err = app.CreateUser(ctx, f.userRepo, "second")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestCreateUserStartsWithTemporaryPassword(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if !f.second.MustChangePassword {
		t.Fatal("новий користувач має бути зобов'язаний змінити пароль")
	}
	if len(f.second.WebhookToken) != 64 {
		t.Fatalf("токен вебхука має бути 256 біт у hex, маємо довжину %d", len(f.second.WebhookToken))
	}
	if _, err := f.users.Authenticate(ctx, "second", f.password); err != nil {
		t.Fatalf("тимчасовий пароль не підійшов: %v", err)
	}
	if _, _, err := app.CreateUser(ctx, f.userRepo, "second"); !errors.Is(err, app.ErrLoginTaken) {
		t.Fatalf("очікували ErrLoginTaken: %v", err)
	}
	if _, _, err := app.CreateUser(ctx, f.userRepo, "two words"); !errors.Is(err, app.ErrInvalidLogin) {
		t.Fatalf("очікували ErrInvalidLogin: %v", err)
	}
}

func TestAuthenticateRejectsBadCredentials(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, err := f.users.Authenticate(ctx, "owner", "wrong"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("невірний пароль: %v", err)
	}
	if _, err := f.users.Authenticate(ctx, "nobody", "x"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("неіснуючий логін: %v", err)
	}
}

func TestChangePassword(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := f.second.ID

	if _, err := f.users.ChangePassword(ctx, id, "wrong", "brand-new-password"); !errors.Is(err, app.ErrWrongCurrentPassword) {
		t.Fatalf("невірний поточний: %v", err)
	}
	if _, err := f.users.ChangePassword(ctx, id, f.password, "short"); !errors.Is(err, app.ErrWeakPassword) {
		t.Fatalf("короткий пароль: %v", err)
	}
	if _, err := f.users.ChangePassword(ctx, id, f.password, strings.Repeat("x", 73)); !errors.Is(err, app.ErrWeakPassword) {
		t.Fatalf("задовгий пароль: %v", err)
	}
	if _, err := f.users.ChangePassword(ctx, id, f.password, f.password); !errors.Is(err, app.ErrSamePassword) {
		t.Fatalf("той самий пароль: %v", err)
	}

	updated, err := f.users.ChangePassword(ctx, id, f.password, "brand-new-password")
	if err != nil {
		t.Fatal(err)
	}
	if updated.MustChangePassword || updated.SessionVersion != f.second.SessionVersion+1 {
		t.Fatalf("прапорець чи версія сесії не оновились: %+v", updated)
	}
	if _, err := f.users.Authenticate(ctx, "second", f.password); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("тимчасовий пароль досі працює: %v", err)
	}
	if _, err := f.users.Authenticate(ctx, "second", "brand-new-password"); err != nil {
		t.Fatalf("новий пароль не працює: %v", err)
	}
}

func TestSetEzbTokenValidatesBeforeSaving(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if err := f.users.SetEzbToken(ctx, f.second.ID, "bad"); !errors.Is(err, app.ErrBadEzbToken) {
		t.Fatalf("поганий токен: %v", err)
	}
	user, _ := f.userRepo.ByID(ctx, f.second.ID)
	if user.EzbToken != "" {
		t.Fatal("поганий токен збережено")
	}

	if err := f.users.SetEzbToken(ctx, f.second.ID, "  good  "); err != nil {
		t.Fatal(err)
	}
	user, _ = f.userRepo.ByID(ctx, f.second.ID)
	if user.EzbToken != "good" || f.waker.woken != 1 {
		t.Fatalf("токен %q, пробуджень %d", user.EzbToken, f.waker.woken)
	}
}

func TestRegisterMonoWebhookUsesOwnURL(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, err := f.users.RegisterMonoWebhook(ctx, f.second.ID, " mono-key "); err != nil {
		t.Fatal(err)
	}
	if f.registrar.token != "mono-key" {
		t.Fatalf("ключ %q", f.registrar.token)
	}
	if want := "https://example.test/mono-webhook/" + f.second.WebhookToken; f.registrar.url != want {
		t.Fatalf("адреса %q, очікували %q", f.registrar.url, want)
	}
	user, _ := f.userRepo.ByID(ctx, f.second.ID)
	if user.MonoWebhookRegisteredAt.IsZero() {
		t.Fatal("час реєстрації не збережено")
	}

	f.registrar.err = app.ErrBadMonoToken
	if _, err := f.users.RegisterMonoWebhook(ctx, f.second.ID, "bad"); !errors.Is(err, app.ErrBadMonoToken) {
		t.Fatalf("очікували ErrBadMonoToken: %v", err)
	}
}

func TestBootstrapOwnerRequiresEnvOnlyOnFirstRun(t *testing.T) {
	f := newFixture(t)
	// власник уже заповнений, тож порожній .env не заважає стартувати
	if err := app.BootstrapOwner(context.Background(), f.userRepo, "", "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestProcessUsesTokenOfJobOwner(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tx := &domain.BankTransaction{Source: domain.SourceMono, ID: "mb_1", AccountID: "acc", Time: time.Unix(1700000000, 0), Amount: -100, MCC: 5411, Description: "Shop"}
	rule := domain.MCCCategoryRule{MCCFrom: 5411, MCCTo: 5411, Direction: domain.DirectionExpense, Priority: 100, EzbCategoryID: "1", Enabled: true}

	if _, err := f.rules.AddRule(ctx, f.second.ID, rule); err != nil {
		t.Fatal(err)
	}
	if err := f.accounts.AddMapping(ctx, f.second.ID, domain.AccountMapping{Source: domain.SourceMono, BankAccountID: "acc", EzbAccountID: "11"}); err != nil {
		t.Fatal(err)
	}
	if err := f.sync.Enqueue(ctx, f.second.ID, tx); err != nil {
		t.Fatal(err)
	}

	pending, err := f.jobs.FetchPending(ctx, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending: %v, %v", pending, err)
	}

	// токена ще немає: транзакція має чекати, а не падати
	if err := f.sync.Process(ctx, &pending[0]); err != nil {
		t.Fatal(err)
	}
	waiting, err := f.jobs.FetchWaiting(ctx, 0, 10)
	if err != nil || len(waiting) != 1 {
		t.Fatalf("waiting: %v, %v", waiting, err)
	}
	if waiting[0].Status != domain.JobWaitingMapping || !strings.Contains(waiting[0].LastError, app.ErrNoEzbToken.Error()) {
		t.Fatalf("джоба %+v", waiting[0])
	}
	if len(*f.sent) != 0 {
		t.Fatalf("транзакцію вже відправлено: %v", *f.sent)
	}

	if err := f.users.SetEzbToken(ctx, f.second.ID, "second-token"); err != nil {
		t.Fatal(err)
	}
	if err := f.sync.Process(ctx, &waiting[0]); err != nil {
		t.Fatal(err)
	}
	if len(*f.sent) != 1 || (*f.sent)[0] != "second-token" {
		t.Fatalf("записано не тим токеном: %v", *f.sent)
	}
}
