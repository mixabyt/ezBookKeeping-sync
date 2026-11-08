-- Повертаємо схему 000008. Дані інших користувачів видаляються: стара схема
-- не вміє їх розрізняти, а змішати їх із даними власника гірше, ніж втратити.

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
        CHECK (status IN ('pending', 'waiting_mapping', 'needs_review', 'done', 'failed', 'skipped')),
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
FROM jobs
WHERE user_id = 1;

DROP TABLE jobs;
ALTER TABLE jobs_old RENAME TO jobs;

CREATE INDEX IF NOT EXISTS idx_jobs_status_id ON jobs (status, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_jobs_source_tx_id ON jobs (source, tx_id);

CREATE TABLE account_mappings_old
(
    source          TEXT     NOT NULL,
    bank_account_id TEXT     NOT NULL,
    ezb_account_id  INTEGER  NOT NULL,
    name            TEXT     NOT NULL DEFAULT '',

    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (source, bank_account_id)
);

INSERT INTO account_mappings_old (source, bank_account_id, ezb_account_id, name, created_at, updated_at)
SELECT source, bank_account_id, ezb_account_id, name, created_at, updated_at
FROM account_mappings
WHERE user_id = 1;

DROP TABLE account_mappings;
ALTER TABLE account_mappings_old RENAME TO account_mappings;

CREATE TABLE mcc_category_rules_old
(
    id              INTEGER  PRIMARY KEY AUTOINCREMENT,
    mcc_from        INTEGER  NOT NULL CHECK (mcc_from BETWEEN 0 AND 9999),
    mcc_to          INTEGER  NOT NULL CHECK (mcc_to BETWEEN 0 AND 9999),
    direction       TEXT     NOT NULL CHECK (direction IN ('expense', 'income')),
    priority        INTEGER  NOT NULL DEFAULT 100,
    ezb_category_id INTEGER  NOT NULL,
    enabled         INTEGER  NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    note            TEXT     NOT NULL DEFAULT '',

    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CHECK (mcc_from <= mcc_to),
    UNIQUE (mcc_from, mcc_to, direction)
);

INSERT INTO mcc_category_rules_old (id, mcc_from, mcc_to, direction, priority, ezb_category_id,
                                    enabled, note, created_at, updated_at)
SELECT id, mcc_from, mcc_to, direction, priority, ezb_category_id, enabled, note, created_at, updated_at
FROM mcc_category_rules
WHERE user_id = 1;

DROP TABLE mcc_category_rules;
ALTER TABLE mcc_category_rules_old RENAME TO mcc_category_rules;

CREATE INDEX IF NOT EXISTS idx_mcc_category_rules_lookup
    ON mcc_category_rules (enabled, direction, mcc_from, mcc_to, priority);

CREATE TABLE merchant_category_rules_old
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

INSERT INTO merchant_category_rules_old (id, mcc, direction, description, ezb_category_id,
                                         enabled, created_at, updated_at)
SELECT id, mcc, direction, description, ezb_category_id, enabled, created_at, updated_at
FROM merchant_category_rules
WHERE user_id = 1;

DROP TABLE merchant_category_rules;
ALTER TABLE merchant_category_rules_old RENAME TO merchant_category_rules;

CREATE TABLE unmapped_keys_old
(
    source      TEXT     NOT NULL,
    key_type    TEXT     NOT NULL CHECK (key_type IN ('mcc', 'bank_category')),
    key         TEXT     NOT NULL,
    direction   TEXT     NOT NULL CHECK (direction IN ('expense', 'income')),
    sample_desc TEXT     NOT NULL DEFAULT '',
    hits        INTEGER  NOT NULL DEFAULT 1,
    first_seen  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (source, key_type, key, direction)
);

INSERT INTO unmapped_keys_old (source, key_type, key, direction, sample_desc, hits, first_seen, last_seen)
SELECT source, key_type, key, direction, sample_desc, hits, first_seen, last_seen
FROM unmapped_keys
WHERE user_id = 1;

DROP TABLE unmapped_keys;
ALTER TABLE unmapped_keys_old RENAME TO unmapped_keys;

CREATE TABLE settings_old
(
    key        TEXT     PRIMARY KEY,
    value      TEXT     NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO settings_old (key, value, updated_at)
SELECT key, value, updated_at
FROM settings
WHERE user_id = 1;

DROP TABLE settings;
ALTER TABLE settings_old RENAME TO settings;

DROP TABLE users;
