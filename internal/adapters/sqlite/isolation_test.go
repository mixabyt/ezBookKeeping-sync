package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"ezbkTools/internal/adapters/sqlite"
	"ezbkTools/internal/adapters/sqlite/sqlitetest"
	"ezbkTools/internal/app"
	"ezbkTools/internal/domain"
)

const (
	alice int64 = 1 // власник із міграції
	bob   int64 = 2
)

func setup(t *testing.T) *sql.DB {
	t.Helper()
	db := sqlitetest.Open(t)
	users := sqlite.NewUser(db)
	ctx := context.Background()
	if err := users.FillOwner(ctx, "alice", "hash-a", "token-a"); err != nil {
		t.Fatal(err)
	}
	created, err := users.Create(ctx, domain.User{Login: "bob", PasswordHash: "hash-b", WebhookToken: "webhook-b"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != bob {
		t.Fatalf("очікували id %d, отримали %d", bob, created.ID)
	}
	return db
}

func addReviewJob(t *testing.T, db *sql.DB, userID int64, txID string) int64 {
	t.Helper()
	jobs := sqlite.NewJob(db)
	ctx := context.Background()
	tx := &domain.BankTransaction{Source: domain.SourceMono, ID: txID, AccountID: "acc", Time: time.Unix(1700000000, 0), Amount: -100, MCC: 5411, Description: "Shop"}
	if err := jobs.Add(ctx, userID, tx); err != nil {
		t.Fatal(err)
	}
	pending, err := jobs.FetchPending(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range pending {
		if job.UserID == userID && job.Tx.ID == txID {
			if err := jobs.MarkNeedsReview(ctx, job.ID); err != nil {
				t.Fatal(err)
			}
			return job.ID
		}
	}
	t.Fatalf("джобу %s не знайдено", txID)
	return 0
}

func TestSameTransactionIDIsPerUser(t *testing.T) {
	db := setup(t)
	jobs := sqlite.NewJob(db)
	ctx := context.Background()
	tx := &domain.BankTransaction{Source: domain.SourceMono, ID: "mb_1", AccountID: "acc", Time: time.Unix(1700000000, 0), Amount: -100, MCC: 5411}

	for _, userID := range []int64{alice, alice, bob} { // другий alice — повтор вебхука
		if err := jobs.Add(ctx, userID, tx); err != nil {
			t.Fatal(err)
		}
	}

	pending, err := jobs.FetchPending(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("очікували по одній джобі на користувача, маємо %d", len(pending))
	}
	if pending[0].UserID == pending[1].UserID {
		t.Fatalf("обидві джоби належать користувачу %d", pending[0].UserID)
	}
}

func TestReviewQueueIsIsolated(t *testing.T) {
	db := setup(t)
	jobs := sqlite.NewJob(db)
	ctx := context.Background()
	aliceJob := addReviewJob(t, db, alice, "mb_a")
	addReviewJob(t, db, bob, "mb_b")

	queue, err := jobs.ListNeedsReview(ctx, bob)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].Tx.ID != "mb_b" {
		t.Fatalf("bob бачить не свою чергу: %+v", queue)
	}

	if err := jobs.Confirm(ctx, bob, aliceJob, "42"); !errors.Is(err, app.ErrJobNotReviewable) {
		t.Fatalf("bob підтвердив чужу джобу: %v", err)
	}
	if n, err := jobs.Skip(ctx, bob, []int64{aliceJob}); err != nil || n != 0 {
		t.Fatalf("bob видалив чужу джобу: n=%d err=%v", n, err)
	}

	queue, err = jobs.ListNeedsReview(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].ID != aliceJob {
		t.Fatalf("джоба alice постраждала: %+v", queue)
	}
	if err := jobs.Confirm(ctx, alice, aliceJob, "42"); err != nil {
		t.Fatalf("alice не змогла підтвердити свою джобу: %v", err)
	}
}

func TestRulesAreIsolated(t *testing.T) {
	db := setup(t)
	categories := sqlite.NewCategory(db)
	ctx := context.Background()
	rule := domain.MCCCategoryRule{MCCFrom: 5411, MCCTo: 5411, Direction: domain.DirectionExpense, Priority: 100, EzbCategoryID: "1", Enabled: true}

	created, err := categories.AddRule(ctx, alice, rule)
	if err != nil {
		t.Fatal(err)
	}
	// той самий MCC у другого користувача — не конфлікт: категорії в нього свої
	if _, err := categories.AddRule(ctx, bob, rule); err != nil {
		t.Fatalf("правило bob конфліктує з правилом alice: %v", err)
	}
	if _, err := categories.AddRule(ctx, alice, rule); !errors.Is(err, app.ErrRuleExists) {
		t.Fatalf("дубль у межах користувача мав дати ErrRuleExists: %v", err)
	}

	if _, err := categories.UpdateRule(ctx, bob, created.ID, "999", true, ""); !errors.Is(err, app.ErrRuleNotFound) {
		t.Fatalf("bob змінив чуже правило: %v", err)
	}
	found, err := categories.EzCategoryRuleByMCC(ctx, alice, 5411, domain.DirectionExpense)
	if err != nil {
		t.Fatal(err)
	}
	if found == nil || found.EzbCategoryID != "1" {
		t.Fatalf("правило alice змінилось: %+v", found)
	}
}

func TestMerchantRulesAndHistoryAreIsolated(t *testing.T) {
	db := setup(t)
	categories := sqlite.NewCategory(db)
	jobs := sqlite.NewJob(db)
	ctx := context.Background()

	created, err := categories.AddMerchantRule(ctx, alice, domain.MerchantRule{MCC: 5818, Direction: domain.DirectionExpense, Description: "Steam", EzbCategoryID: "7", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if rule, err := categories.MerchantRule(ctx, bob, 5818, domain.DirectionExpense, "Steam"); err != nil || rule != nil {
		t.Fatalf("bob бачить чуже правило продавця: %+v, %v", rule, err)
	}
	if _, err := categories.UpdateMerchantRule(ctx, bob, created.ID, "9", true); !errors.Is(err, app.ErrRuleNotFound) {
		t.Fatalf("bob змінив чуже правило продавця: %v", err)
	}

	jobID := addReviewJob(t, db, alice, "mb_h")
	if _, err := db.ExecContext(ctx, `UPDATE jobs SET mcc = 5818, description = 'Steam' WHERE id = ?`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := jobs.MarkDone(ctx, jobID, "7"); err != nil {
		t.Fatal(err)
	}
	if last, err := categories.LastCategoryFor(ctx, alice, 5818, domain.DirectionExpense, "Steam"); err != nil || last != "7" {
		t.Fatalf("alice не бачить свою історію: %q, %v", last, err)
	}
	if last, err := categories.LastCategoryFor(ctx, bob, 5818, domain.DirectionExpense, "Steam"); err != nil || last != "" {
		t.Fatalf("bob бачить чужу історію: %q, %v", last, err)
	}
}

func TestUnmappedResolvedUsesOwnRules(t *testing.T) {
	db := setup(t)
	categories := sqlite.NewCategory(db)
	ctx := context.Background()
	key := domain.UnmappedKey{Source: domain.SourceMono, KeyType: domain.KeyMCC, Key: "5411", Direction: domain.DirectionExpense}

	for _, userID := range []int64{alice, bob} {
		if err := categories.TrackUnmapped(ctx, userID, key); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := categories.AddRule(ctx, alice, domain.MCCCategoryRule{MCCFrom: 5411, MCCTo: 5411, Direction: domain.DirectionExpense, Priority: 100, EzbCategoryID: "1", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	bobKeys, err := categories.ListUnmapped(ctx, bob, domain.UnmappedFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(bobKeys) != 1 || bobKeys[0].Resolved {
		t.Fatalf("правило alice закрило борг bob: %+v", bobKeys)
	}
	aliceKeys, err := categories.ListUnmapped(ctx, alice, domain.UnmappedFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(aliceKeys) != 1 || !aliceKeys[0].Resolved {
		t.Fatalf("у alice ключ мав стати resolved: %+v", aliceKeys)
	}
}

func TestAccountMappingsAndSettingsAreIsolated(t *testing.T) {
	db := setup(t)
	accounts := sqlite.NewAccount(db)
	settings := sqlite.NewSettings(db)
	ctx := context.Background()

	if err := accounts.AddMapping(ctx, alice, domain.AccountMapping{Source: domain.SourceMono, BankAccountID: "acc", EzbAccountID: "11"}); err != nil {
		t.Fatal(err)
	}
	if m, err := accounts.EzAccountByBank(ctx, bob, domain.SourceMono, "acc"); err != nil || m != nil {
		t.Fatalf("bob бачить чужий мапінг: %+v, %v", m, err)
	}

	if err := settings.SetMappingMode(ctx, bob, domain.ModeManual); err != nil {
		t.Fatal(err)
	}
	if mode, err := settings.MappingMode(ctx, alice); err != nil || mode != domain.ModeMCC {
		t.Fatalf("режим bob вплинув на alice: %q, %v", mode, err)
	}
	if mode, err := settings.MappingMode(ctx, bob); err != nil || mode != domain.ModeManual {
		t.Fatalf("режим bob не збережено: %q, %v", mode, err)
	}
}

func TestCreateUserRejectsTakenLogin(t *testing.T) {
	db := setup(t)
	_, err := sqlite.NewUser(db).Create(context.Background(), domain.User{Login: "bob", PasswordHash: "x", WebhookToken: "other"})
	if !errors.Is(err, app.ErrLoginTaken) {
		t.Fatalf("очікували ErrLoginTaken: %v", err)
	}
}

func TestFillOwnerDoesNotOverwrite(t *testing.T) {
	db := setup(t)
	users := sqlite.NewUser(db)
	ctx := context.Background()

	if err := users.FillOwner(ctx, "mallory", "hash-m", "token-m"); err != nil {
		t.Fatal(err)
	}
	owner, err := users.ByID(ctx, domain.OwnerUserID)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Login != "alice" || owner.PasswordHash != "hash-a" {
		t.Fatalf("дані власника перезаписано: %+v", owner)
	}
}
