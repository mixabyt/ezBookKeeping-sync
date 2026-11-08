// Package sqlitetest піднімає для тестів тимчасову БД зі справжніми міграціями.
package sqlitetest

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // драйвер "sqlite"
)

// Open створює БД у тимчасовому каталозі тесту і проганяє всі up-міграції.
// Рядок власника заповнюється одразу: тестам потрібна вже робоча БД.
func Open(t *testing.T) *sql.DB {
	t.Helper()

	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "test.db")) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	version := Migrate(t, db)
	if _, err := db.ExecContext(context.Background(),
		`CREATE TABLE schema_migrations (version INTEGER NOT NULL, dirty INTEGER NOT NULL);
		 INSERT INTO schema_migrations (version, dirty) VALUES (?, 0)`, version); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	return db
}

// Migrate виконує всі up-міграції по черзі й повертає номер останньої.
func Migrate(t *testing.T, db *sql.DB) int {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("не вдалося визначити шлях до міграцій")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "database", "migrations")

	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("не знайдено міграцій у %s: %v", dir, err)
	}
	sort.Strings(files)

	for _, file := range files {
		script, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if _, err := db.ExecContext(context.Background(), string(script)); err != nil {
			t.Fatalf("migration %s: %v", filepath.Base(file), err)
		}
	}

	last, _, _ := strings.Cut(filepath.Base(files[len(files)-1]), "_")
	version, err := strconv.Atoi(last)
	if err != nil {
		t.Fatalf("номер міграції %q: %v", last, err)
	}
	return version
}
