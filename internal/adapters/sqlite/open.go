package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // драйвер "sqlite"
)

// Open відкриває БД з налаштуваннями, без яких SQLite не витримує паралельних запитів.
//
// Воркер пише в БД у той самий момент, коли PWA читає списки (після кожного мапінгу
// він прокидається саме тоді, коли UI перезавантажує дані). У режимі за замовчуванням
// такий читач одразу отримує "database is locked (SQLITE_BUSY)":
//   - journal_mode=WAL — читачі не блокуються записом і навпаки;
//   - busy_timeout=5000 — два писачі одночасно: другий чекає до 5 с, а не падає.
//
// WAL створює поруч файли <db>-wal і <db>-shm. Робити бекап через cp при запущеному
// сервісі не можна — частина даних може лежати ще в -wal; правильно: sqlite3 <db> ".backup <file>".
func Open(path string) (*sql.DB, error) {
	// порожній шлях SQLite мовчки перетворює на тимчасову БД без таблиць:
	// сервіс стартував би "успішно", а кожен вебхук падав би з "no such table: jobs"
	if path == "" {
		return nil, errors.New("DB_DATABASE не задано: перевірте, що .env завантажився")
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}

	// перевіряємо одразу: інакше помилка шляху вилізе лише на першому запиті
	// у вигляді загадкового "no such table: jobs"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}

	if err := checkSchema(ctx, db, path); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// requiredSchemaVersion — остання міграція в database/migrations, без якої код не працює.
// Піднімати разом з кожною новою міграцією.
const requiredSchemaVersion = 6

// checkSchema не дає стартувати на БД без міграцій. Відсутній файл SQLite створює
// порожнім, і без цієї перевірки сервіс "успішно" запускався б, а кожен запит
// падав би з "no such table". Те саме з недомігрованою БД: без 000006 перший же
// перехід джоби в waiting_mapping впирається в старий CHECK.
func checkSchema(ctx context.Context, db *sql.DB, path string) error {
	migrateHint := fmt.Sprintf(`migrate -path database/migrations -database "sqlite3://%s" up`, path)

	var tables int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`,
	).Scan(&tables); err != nil {
		return fmt.Errorf("check schema %s: %w", path, err)
	}
	if tables == 0 {
		return fmt.Errorf("БД %s без міграцій (немає schema_migrations) — запустіть: %s", path, migrateHint)
	}

	var (
		version int
		dirty   bool
	)
	if err := db.QueryRowContext(ctx, `SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty); err != nil {
		return fmt.Errorf("БД %s: не вдалося прочитати версію схеми — запустіть: %s (%w)", path, migrateHint, err)
	}
	if dirty {
		return fmt.Errorf("БД %s: міграція %d зупинилась на півдорозі (dirty) — розберіться вручну і зробіть migrate force", path, version)
	}
	if version < requiredSchemaVersion {
		return fmt.Errorf("БД %s на версії схеми %d, потрібна %d — запустіть: %s", path, version, requiredSchemaVersion, migrateHint)
	}
	return nil
}
