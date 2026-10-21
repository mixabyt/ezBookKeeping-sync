-- Повертаємо старий CHECK. Джоби, що чекали мапінгу, знову стають pending —
-- без нового статусу інакше вони б порушили констрейнт.
UPDATE jobs SET status = 'pending' WHERE status = 'waiting_mapping';

CREATE TABLE jobs_old
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
        CHECK (status IN ('pending', 'done', 'failed')),
    attempts       INTEGER  NOT NULL DEFAULT 0,
    last_error     TEXT     NOT NULL DEFAULT '',

    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO jobs_old (id, source, tx_id, account_id, tx_time, amount, mcc, description, hold,
                      status, attempts, last_error, created_at, updated_at)
SELECT id, source, tx_id, account_id, tx_time, amount, mcc, description, hold,
       status, attempts, last_error, created_at, updated_at
FROM jobs;

DROP TABLE jobs;
ALTER TABLE jobs_old RENAME TO jobs;

CREATE INDEX IF NOT EXISTS idx_jobs_status_id ON jobs (status, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_jobs_source_tx_id ON jobs (source, tx_id);
