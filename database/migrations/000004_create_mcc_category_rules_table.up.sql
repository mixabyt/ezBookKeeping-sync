-- Крок 2 категоризації — правила: MCC + напрямок → категорія ezBookkeeping.
-- Однакові для всіх банків. Один MCC — це mcc_from = mcc_to.
-- Діапазони можуть перетинатися (5811-5814 і окремо 5812): перемагає менший priority.
-- Жодне правило не підійшло — дефолтна категорія для напрямку (з конфігу).
CREATE TABLE IF NOT EXISTS mcc_category_rules
(
    id              INTEGER  PRIMARY KEY AUTOINCREMENT,
    mcc_from        INTEGER  NOT NULL CHECK (mcc_from BETWEEN 0 AND 9999),
    mcc_to          INTEGER  NOT NULL CHECK (mcc_to BETWEEN 0 AND 9999),
    direction       TEXT     NOT NULL CHECK (direction IN ('expense', 'income')),
    priority        INTEGER  NOT NULL DEFAULT 100,
    ezb_category_id INTEGER  NOT NULL,
    enabled         INTEGER  NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    note            TEXT     NOT NULL DEFAULT '', -- для людини: "ресторани й кафе"

    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CHECK (mcc_from <= mcc_to),
    UNIQUE (mcc_from, mcc_to, direction)
);

CREATE INDEX IF NOT EXISTS idx_mcc_category_rules_lookup
    ON mcc_category_rules (enabled, direction, mcc_from, mcc_to, priority);
