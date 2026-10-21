package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"ezbkTools/internal/domain"
)

// Account — мапінг банківського рахунку на рахунок ezBookkeeping (таблиця account_mappings).
type Account struct {
	db *sql.DB
}

func NewAccount(db *sql.DB) *Account {
	return &Account{db: db}
}

// EzAccountByBank повертає nil, nil, якщо мапінгу немає — це не помилка,
// а сигнал для app поставити джобу в очікування.
func (a *Account) EzAccountByBank(ctx context.Context, source domain.Source, bankAccountID string) (*domain.AccountMapping, error) {
	query := `
		SELECT source, bank_account_id, ezb_account_id, name
		FROM account_mappings
		WHERE source = ? AND bank_account_id = ?
	`

	var (
		mapping      domain.AccountMapping
		ezbAccountID int64
	)
	err := a.db.QueryRowContext(ctx, query, source, bankAccountID).Scan(
		&mapping.Source,
		&mapping.BankAccountID,
		&ezbAccountID,
		&mapping.Name,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("account mapping %s/%s: %w", source, bankAccountID, err)
	}

	// ezb_account_id у схемі INTEGER, а в API ezBookkeeping це рядок із великим числом
	mapping.EzbAccountID = fmt.Sprint(ezbAccountID)
	return &mapping, nil
}

// AddMapping створює мапінг або перезаписує наявний: повторне збереження з UI
// має виправляти помилку ("обрав не той рахунок"), а не падати з конфліктом.
func (a *Account) AddMapping(ctx context.Context, mapping domain.AccountMapping) error {
	query := `
		INSERT INTO account_mappings (source, bank_account_id, ezb_account_id, name)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (source, bank_account_id) DO UPDATE SET
			ezb_account_id = excluded.ezb_account_id,
			name           = excluded.name,
			updated_at     = CURRENT_TIMESTAMP
	`
	_, err := a.db.ExecContext(ctx, query,
		mapping.Source,
		mapping.BankAccountID,
		mapping.EzbAccountID,
		mapping.Name,
	)
	if err != nil {
		return fmt.Errorf("add account mapping %s/%s: %w", mapping.Source, mapping.BankAccountID, err)
	}
	return nil
}

func (a *Account) ListMappings(ctx context.Context) ([]domain.AccountMapping, error) {
	query := `
		SELECT source, bank_account_id, ezb_account_id, name
		FROM account_mappings
		ORDER BY source, bank_account_id
	`
	rows, err := a.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list account mappings: %w", err)
	}
	defer rows.Close()

	mappings := make([]domain.AccountMapping, 0)
	for rows.Next() {
		var (
			mapping      domain.AccountMapping
			ezbAccountID int64
		)
		if err := rows.Scan(&mapping.Source, &mapping.BankAccountID, &ezbAccountID, &mapping.Name); err != nil {
			return nil, fmt.Errorf("scan account mapping: %w", err)
		}
		mapping.EzbAccountID = fmt.Sprint(ezbAccountID)
		mappings = append(mappings, mapping)
	}
	return mappings, rows.Err()
}
