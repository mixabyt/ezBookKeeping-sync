-- Кілька користувачів: у кожного свій пароль, токен ezBookkeeping, адреса вебхука і свої дані.
--
-- Власник — рядок id = 1. Усе, що вже є в БД, належить йому. Логін і хеш тут не відомі
-- (вони в .env), тож рядок створюється порожнім, а заповнює його сервіс при першому старті.
--
-- webhook_token — випадковий секрет у шляху вебхука. Monobank підписує вебхуки всіх клієнтів
-- одним ключем: підпис доводить «це від банку», але не «це для цього користувача».
CREATE TABLE users
(
    id                         INTEGER  PRIMARY KEY AUTOINCREMENT,
    login                      TEXT     NOT NULL UNIQUE,
    password_hash              TEXT     NOT NULL DEFAULT '', -- bcrypt
    must_change_password       INTEGER  NOT NULL DEFAULT 0 CHECK (must_change_password IN (0, 1)),
    session_version            INTEGER  NOT NULL DEFAULT 1,  -- зміна пароля піднімає її і гасить старі сесії
    webhook_token              TEXT     NOT NULL UNIQUE CHECK (webhook_token <> ''),
    ezb_token                  TEXT     NOT NULL DEFAULT '', -- порожній — транзакції чекають у waiting_mapping
    mono_webhook_registered_at INTEGER  NOT NULL DEFAULT 0,  -- unix seconds, 0 — ще не реєстрували

    created_at                 DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                 DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO users (id, login, webhook_token) VALUES (1, '', lower(hex(randomblob(32))));

-- Далі перебудова таблиць, з тих самих причин і в тому ж порядку, що в 000006.
-- user_id без DEFAULT навмисно: забутий user_id має дати помилку, а не тихий запис у дані власника.

CREATE TABLE jobs_new
(
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER  NOT NULL REFERENCES users (id),

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

INSERT INTO jobs_new (id, user_id, source, tx_id, account_id, tx_time, amount, mcc, description, hold,
                      status, attempts, last_error, ezb_category_id, created_at, updated_at)
SELECT id, 1, source, tx_id, account_id, tx_time, amount, mcc, description, hold,
       status, attempts, last_error, ezb_category_id, created_at, updated_at
FROM jobs;

DROP TABLE jobs;
ALTER TABLE jobs_new RENAME TO jobs;

-- idx_jobs_status_id — для воркера, він іде по всіх користувачах; idx_jobs_user_status — для PWA
CREATE INDEX IF NOT EXISTS idx_jobs_status_id ON jobs (status, id);
CREATE INDEX IF NOT EXISTS idx_jobs_user_status ON jobs (user_id, status, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_jobs_user_source_tx_id ON jobs (user_id, source, tx_id);

CREATE TABLE account_mappings_new
(
    user_id         INTEGER  NOT NULL REFERENCES users (id),
    source          TEXT     NOT NULL,
    bank_account_id TEXT     NOT NULL,
    ezb_account_id  INTEGER  NOT NULL,
    name            TEXT     NOT NULL DEFAULT '',

    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (user_id, source, bank_account_id)
);

INSERT INTO account_mappings_new (user_id, source, bank_account_id, ezb_account_id, name, created_at, updated_at)
SELECT 1, source, bank_account_id, ezb_account_id, name, created_at, updated_at
FROM account_mappings;

DROP TABLE account_mappings;
ALTER TABLE account_mappings_new RENAME TO account_mappings;

CREATE TABLE mcc_category_rules_new
(
    id              INTEGER  PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER  NOT NULL REFERENCES users (id),
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
    UNIQUE (user_id, mcc_from, mcc_to, direction)
);

INSERT INTO mcc_category_rules_new (id, user_id, mcc_from, mcc_to, direction, priority, ezb_category_id,
                                    enabled, note, created_at, updated_at)
SELECT id, 1, mcc_from, mcc_to, direction, priority, ezb_category_id, enabled, note, created_at, updated_at
FROM mcc_category_rules;

DROP TABLE mcc_category_rules;
ALTER TABLE mcc_category_rules_new RENAME TO mcc_category_rules;

CREATE INDEX IF NOT EXISTS idx_mcc_category_rules_lookup
    ON mcc_category_rules (user_id, enabled, direction, mcc_from, mcc_to, priority);

CREATE TABLE merchant_category_rules_new
(
    id              INTEGER  PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER  NOT NULL REFERENCES users (id),
    mcc             INTEGER  NOT NULL CHECK (mcc BETWEEN 0 AND 9999),
    direction       TEXT     NOT NULL CHECK (direction IN ('expense', 'income')),
    description     TEXT     NOT NULL CHECK (description <> ''),
    ezb_category_id INTEGER  NOT NULL,
    enabled         INTEGER  NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),

    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (user_id, mcc, direction, description)
);

INSERT INTO merchant_category_rules_new (id, user_id, mcc, direction, description, ezb_category_id,
                                         enabled, created_at, updated_at)
SELECT id, 1, mcc, direction, description, ezb_category_id, enabled, created_at, updated_at
FROM merchant_category_rules;

DROP TABLE merchant_category_rules;
ALTER TABLE merchant_category_rules_new RENAME TO merchant_category_rules;

CREATE TABLE unmapped_keys_new
(
    user_id     INTEGER  NOT NULL REFERENCES users (id),
    source      TEXT     NOT NULL,
    key_type    TEXT     NOT NULL CHECK (key_type IN ('mcc', 'bank_category')),
    key         TEXT     NOT NULL,
    direction   TEXT     NOT NULL CHECK (direction IN ('expense', 'income')),
    sample_desc TEXT     NOT NULL DEFAULT '',
    hits        INTEGER  NOT NULL DEFAULT 1,
    first_seen  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (user_id, source, key_type, key, direction)
);

INSERT INTO unmapped_keys_new (user_id, source, key_type, key, direction, sample_desc, hits, first_seen, last_seen)
SELECT 1, source, key_type, key, direction, sample_desc, hits, first_seen, last_seen
FROM unmapped_keys;

DROP TABLE unmapped_keys;
ALTER TABLE unmapped_keys_new RENAME TO unmapped_keys;

CREATE TABLE settings_new
(
    user_id    INTEGER  NOT NULL REFERENCES users (id),
    key        TEXT     NOT NULL,
    value      TEXT     NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (user_id, key)
);

INSERT INTO settings_new (user_id, key, value, updated_at)
SELECT 1, key, value, updated_at
FROM settings;

DROP TABLE settings;
ALTER TABLE settings_new RENAME TO settings;
