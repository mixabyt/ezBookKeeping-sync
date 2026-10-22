-- Режими мапінгу категорій (налаштування в PWA):
--   mcc      — лише правило MCC (як було);
--   merchant — спершу правило продавця (MCC + напрямок + опис), потім правило MCC;
--   manual   — кожна транзакція чекає підтвердження людиною (статус needs_review).
CREATE TABLE IF NOT EXISTS settings
(
    key        TEXT     PRIMARY KEY,
    value      TEXT     NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- після деплою поведінка не змінюється, поки режим не перемкнуть в UI
INSERT OR IGNORE INTO settings (key, value) VALUES ('mapping_mode', 'mcc');

-- Правило продавця: один MCC покриває різне (5818 — і Preply, і Steam),
-- тому категорія задається на пару MCC + опис транзакції з Monobank.
-- Опис порівнюється точно (після trim): lower() у SQLite працює лише з ASCII.
CREATE TABLE IF NOT EXISTS merchant_category_rules
(
    id              INTEGER  PRIMARY KEY AUTOINCREMENT,
    mcc             INTEGER  NOT NULL CHECK (mcc BETWEEN 0 AND 9999),
    direction       TEXT     NOT NULL CHECK (direction IN ('expense', 'income')),
    description     TEXT     NOT NULL CHECK (description <> ''),
    ezb_category_id INTEGER  NOT NULL,
    enabled         INTEGER  NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),

    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (mcc, direction, description)
);

-- jobs: новий статус needs_review (режим manual) і колонка ezb_category_id —
-- категорія, підтверджена людиною або фактично використана при записі в ezBookkeeping
-- (з неї ж береться пропозиція «як минулого разу»).
-- Перебудова таблиці — з тих самих причин і в тому ж порядку, що в 000006.
CREATE TABLE jobs_new
(
    id              INTEGER PRIMARY KEY AUTOINCREMENT,

    source          TEXT     NOT NULL,
    tx_id           TEXT     NOT NULL,
    account_id      TEXT     NOT NULL,
    tx_time         INTEGER  NOT NULL,
    amount          INTEGER  NOT NULL,
    mcc             INTEGER  NOT NULL,
    description     TEXT     NOT NULL DEFAULT '',
    hold            INTEGER  NOT NULL DEFAULT 0,

    status          TEXT     NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'waiting_mapping', 'needs_review', 'done', 'failed')),
    attempts        INTEGER  NOT NULL DEFAULT 0,
    last_error      TEXT     NOT NULL DEFAULT '',
    ezb_category_id TEXT     NOT NULL DEFAULT '',

    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO jobs_new (id, source, tx_id, account_id, tx_time, amount, mcc, description, hold,
                      status, attempts, last_error, created_at, updated_at)
SELECT id, source, tx_id, account_id, tx_time, amount, mcc, description, hold,
       status, attempts, last_error, created_at, updated_at
FROM jobs;

DROP TABLE jobs;
ALTER TABLE jobs_new RENAME TO jobs;

CREATE INDEX IF NOT EXISTS idx_jobs_status_id ON jobs (status, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_jobs_source_tx_id ON jobs (source, tx_id);
