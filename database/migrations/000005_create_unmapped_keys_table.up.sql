-- Ключі, для яких не знайшлося мапінгу — підказка, що дописати:
--   key_type = 'bank_category' — нормалізація не знайшла MCC (немає рядка в bank_category_mcc);
--   key_type = 'mcc'           — MCC є, але жодне правило в mcc_category_rules не підійшло.
-- Транзакція при цьому все одно записується в дефолтну категорію.
CREATE TABLE IF NOT EXISTS unmapped_keys
(
    source      TEXT     NOT NULL,
    key_type    TEXT     NOT NULL CHECK (key_type IN ('mcc', 'bank_category')),
    key         TEXT     NOT NULL, -- '5411' або 'Супермаркети та продукти'
    direction   TEXT     NOT NULL CHECK (direction IN ('expense', 'income')),
    sample_desc TEXT     NOT NULL DEFAULT '', -- опис однієї з транзакцій, щоб зрозуміти, що це
    hits        INTEGER  NOT NULL DEFAULT 1,
    first_seen  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (source, key_type, key, direction)
);
