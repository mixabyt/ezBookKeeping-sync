-- Рахунок у банку → рахунок ezBookkeeping.
-- Немає рядка для рахунку — транзакцію не записуємо (дефолтного рахунку немає навмисно:
-- запис не на той рахунок ламає баланс).
CREATE TABLE IF NOT EXISTS account_mappings
(
    source          TEXT     NOT NULL, -- 'mono' | 'privat' | ...
    bank_account_id TEXT     NOT NULL,
    ezb_account_id  INTEGER  NOT NULL,
    name            TEXT     NOT NULL DEFAULT '', -- для людини: "Чорна картка UAH"

    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (source, bank_account_id)
);
