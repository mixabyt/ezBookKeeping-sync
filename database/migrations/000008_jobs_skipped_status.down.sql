-- Повертаємо jobs до схеми 000007. Пропущені джоби стають failed, а не зникають:
-- рядок має лишитись, інакше повтор вебхука створив би транзакцію знову,
-- а failed воркер не бере — у ezBookkeeping вона так і не піде.
UPDATE jobs SET status = 'failed', last_error = 'skipped' WHERE status = 'skipped';

CREATE TABLE jobs_old
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

INSERT INTO jobs_old (id, source, tx_id, account_id, tx_time, amount, mcc, description, hold,
                      status, attempts, last_error, ezb_category_id, created_at, updated_at)
SELECT id, source, tx_id, account_id, tx_time, amount, mcc, description, hold,
       status, attempts, last_error, ezb_category_id, created_at, updated_at
FROM jobs;

DROP TABLE jobs;
ALTER TABLE jobs_old RENAME TO jobs;

CREATE INDEX IF NOT EXISTS idx_jobs_status_id ON jobs (status, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_jobs_source_tx_id ON jobs (source, tx_id);
