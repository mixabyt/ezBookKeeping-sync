package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"ezbkTools/internal/domain"
)

const settingMappingMode = "mapping_mode"

// Settings — налаштування, що змінюються з PWA (таблиця settings, ключ-значення).
type Settings struct {
	db *sql.DB
}

func NewSettings(db *sql.DB) *Settings {
	return &Settings{db: db}
}

// MappingMode — рядка немає (БД змінили руками) → mcc, тобто поведінка як до режимів.
func (s *Settings) MappingMode(ctx context.Context) (domain.MappingMode, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, settingMappingMode).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ModeMCC, nil
		}
		return "", fmt.Errorf("read mapping mode: %w", err)
	}
	return domain.MappingMode(value), nil
}

func (s *Settings) SetMappingMode(ctx context.Context, mode domain.MappingMode) error {
	query := `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`
	if _, err := s.db.ExecContext(ctx, query, settingMappingMode, string(mode)); err != nil {
		return fmt.Errorf("set mapping mode: %w", err)
	}
	return nil
}
