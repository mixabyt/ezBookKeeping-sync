package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"ezbkTools/internal/app"
	"ezbkTools/internal/domain"
)

type Category struct {
	db *sql.DB
}

func NewCategory(db *sql.DB) *Category {
	return &Category{
		db: db,
	}
}

func (c *Category) EzCategoryRuleByMCC(ctx context.Context, mcc int) (*domain.MCCCategoryRule, error) {
	query := `
		SELECT
			id,
			mcc_from,
			mcc_to,
			direction,
			priority,
			ezb_category_id,
			enabled,
			note
		FROM mcc_category_rules
		WHERE enabled = 1
			AND mcc_from <= ?
			AND mcc_to >= ?
		ORDER BY priority ASC
		LIMIT 1
	`

	var rule domain.MCCCategoryRule
	var direction string
	var enabled int
	var categoryID int64

	err := c.db.QueryRowContext(ctx, query, mcc, mcc).Scan(
		&rule.ID,
		&rule.MCCFrom,
		&rule.MCCTo,
		&direction,
		&rule.Priority,
		&categoryID,
		&enabled,
		&rule.Note,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	rule.Direction = domain.Direction(direction)
	rule.EzbCategoryID = fmt.Sprint(categoryID)
	rule.Enabled = enabled == 1

	return &rule, nil
}

func (c *Category) MccByBankCategory(ctx context.Context, source domain.Source, bankCategory string) (*domain.BankCategoryMCC, error) {
	query := `
		SELECT
			source,
			bank_category,
			mcc
		FROM bank_category_mcc
		WHERE source = ?
			AND bank_category = ?
		LIMIT 1
	`

	var category domain.BankCategoryMCC

	err := c.db.QueryRowContext(ctx, query, source, bankCategory).Scan(
		&category.Source,
		&category.BankCategory,
		&category.MCC,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	return &category, nil
}

// TrackUnmapped записує ключ, для якого немає мапінгу, або збільшує лічильник,
// якщо такий ключ уже бачили. sample_desc зберігаємо лише перший — як зразок,
// за яким потім зрозуміло, що це взагалі було.
func (c *Category) TrackUnmapped(ctx context.Context, key domain.UnmappedKey) error {
	query := `
		INSERT INTO unmapped_keys (source, key_type, "key", direction, sample_desc, hits, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (source, key_type, "key", direction) DO UPDATE SET
			hits      = hits + 1,
			last_seen = CURRENT_TIMESTAMP,
			sample_desc = CASE
				WHEN unmapped_keys.sample_desc = '' THEN excluded.sample_desc
				ELSE unmapped_keys.sample_desc
			END
	`

	_, err := c.db.ExecContext(ctx, query,
		key.Source,
		key.KeyType,
		key.Key,
		key.Direction,
		key.SampleDesc,
	)
	if err != nil {
		return fmt.Errorf("track unmapped key %s/%s: %w", key.KeyType, key.Key, err)
	}
	return nil
}

// ListUnmapped віддає ключі без мапінгу для UI.
//
// resolved рахується на льоту: рядки в unmapped_keys не видаляються, тому без
// цієї ознаки список показував би вже замаплені ключі як борг. Для bank_category
// правил не існує, тому там resolved завжди 0.
func (c *Category) ListUnmapped(ctx context.Context, filter domain.UnmappedFilter) ([]domain.UnmappedKey, error) {
	// resolved рахується у підзапиті, бо фільтрувати за ним у WHERE того ж
	// рівня не можна (а HAVING без GROUP BY SQLite не приймає)
	query := `
		SELECT source, key_type, "key", direction, sample_desc, hits, first_seen, last_seen, resolved
		FROM (
			SELECT
				u.source,
				u.key_type,
				u."key"        AS "key",
				u.direction,
				u.sample_desc,
				u.hits,
				u.first_seen,
				u.last_seen,
				EXISTS (
					SELECT 1 FROM mcc_category_rules r
					WHERE r.enabled = 1
						AND u.key_type = 'mcc'
						AND r.direction = u.direction
						AND CAST(u."key" AS INTEGER) BETWEEN r.mcc_from AND r.mcc_to
				) AS resolved
			FROM unmapped_keys u
			WHERE (? = '' OR u.key_type = ?)
		)
		WHERE (? = 0 OR resolved = 0)
		ORDER BY resolved ASC, hits DESC, last_seen DESC
		LIMIT ?
	`

	onlyPending := 0
	if filter.OnlyPending {
		onlyPending = 1
	}

	rows, err := c.db.QueryContext(ctx, query,
		string(filter.KeyType), string(filter.KeyType),
		onlyPending,
		filter.Limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list unmapped keys: %w", err)
	}
	defer rows.Close()

	keys := make([]domain.UnmappedKey, 0, filter.Limit)
	for rows.Next() {
		var key domain.UnmappedKey
		if err := rows.Scan(
			&key.Source,
			&key.KeyType,
			&key.Key,
			&key.Direction,
			&key.SampleDesc,
			&key.Hits,
			&key.FirstSeen,
			&key.LastSeen,
			&key.Resolved,
		); err != nil {
			return nil, fmt.Errorf("scan unmapped key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unmapped keys: %w", err)
	}
	return keys, nil
}

// AddRule додає правило MCC → категорія.
// Порушення UNIQUE (mcc_from, mcc_to, direction) перетворюємо на app.ErrRuleExists,
// щоб API віддав 409, а не 500: для користувача це не збій, а "таке правило вже є".
func (c *Category) AddRule(ctx context.Context, rule domain.MCCCategoryRule) (*domain.MCCCategoryRule, error) {
	query := `
		INSERT INTO mcc_category_rules (mcc_from, mcc_to, direction, priority, ezb_category_id, enabled, note)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`

	enabled := 1
	if !rule.Enabled {
		enabled = 0
	}

	err := c.db.QueryRowContext(ctx, query,
		rule.MCCFrom,
		rule.MCCTo,
		rule.Direction,
		rule.Priority,
		rule.EzbCategoryID,
		enabled,
		rule.Note,
	).Scan(&rule.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, fmt.Errorf("%w: %d–%d %s", app.ErrRuleExists, rule.MCCFrom, rule.MCCTo, rule.Direction)
		}
		return nil, fmt.Errorf("add mcc rule %d-%d: %w", rule.MCCFrom, rule.MCCTo, err)
	}
	return &rule, nil
}
