package sqlite

import (
	"context"
	"database/sql"
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
	return db, nil
}
