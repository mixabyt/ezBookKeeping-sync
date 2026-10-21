-- Новий статус waiting_mapping: джоба чекає, поки людина не задасть категорію і рахунок.
-- SQLite не вміє змінювати CHECK, тому таблицю перебудовуємо. Порядок важливий:
-- нова таблиця під тимчасовим іменем → копія даних → DROP старої (разом з її індексами)
-- → перейменування → індекси заново. Якщо спершу перейменувати стару, її індекси
-- переїдуть разом з нею і зайнятимуть ті самі імена.
CREATE TABLE jobs_new
(
    id             INTEGER PRIMARY KEY AUTOINCREMENT,

    source         TEXT     NOT NULL,
    tx_id          TEXT     NOT NULL,
    account_id     TEXT     NOT NULL,
    tx_time        INTEGER  NOT NULL,
    amount         INTEGER  NOT NULL,
    mcc            INTEGER  NOT NULL,
    description    TEXT     NOT NULL DEFAULT '',
    hold           INTEGER  NOT NULL DEFAULT 0,

    status         TEXT     NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'waiting_mapping', 'done', 'failed')),
    attempts       INTEGER  NOT NULL DEFAULT 0,
    last_error     TEXT     NOT NULL DEFAULT '',

    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
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
