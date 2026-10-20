-- Крок 1 категоризації — нормалізація: банківська категорія → MCC.
-- Для банків без MCC (напр. Приват). Для Monobank рядків немає: MCC він дає сам.
-- Нормалізація відбувається до запису в jobs; не знайшли — ключ іде в unmapped_keys.
CREATE TABLE IF NOT EXISTS bank_category_mcc
(
    source        TEXT     NOT NULL, -- 'privat' | ...
    bank_category TEXT     NOT NULL, -- 'Супермаркети та продукти'
    mcc           INTEGER  NOT NULL CHECK (mcc BETWEEN 0 AND 9999),

    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (source, bank_category)
);
