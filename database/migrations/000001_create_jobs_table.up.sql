CREATE TABLE IF NOT EXISTS jobs
(
    id             INTEGER PRIMARY KEY AUTOINCREMENT,

    -- domain.BankTransaction
    source         TEXT     NOT NULL, -- 'mono' | 'privat' | ...
    tx_id          TEXT     NOT NULL, -- ID транзакції в банку
    account_id     TEXT     NOT NULL,
    tx_time        INTEGER  NOT NULL, -- unix seconds
    amount         INTEGER  NOT NULL, -- копійки, мінус — витрата
    mcc            INTEGER  NOT NULL, -- для банків без MCC — вже після нормалізації
    description    TEXT     NOT NULL DEFAULT '',
    hold           INTEGER  NOT NULL DEFAULT 0, -- 0/1

    -- стан джоби
    status         TEXT     NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'done', 'failed')),
    attempts       INTEGER  NOT NULL DEFAULT 0,
    last_error     TEXT     NOT NULL DEFAULT '',

    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_jobs_status_id ON jobs (status, id);

-- Ідемпотентність: той самий вебхук двічі не створить другу джобу.
-- ID транзакції унікальний лише в межах банку, тому ключ — пара.
CREATE UNIQUE INDEX IF NOT EXISTS ux_jobs_source_tx_id ON jobs (source, tx_id);
