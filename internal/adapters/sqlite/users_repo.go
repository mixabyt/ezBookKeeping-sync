package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"ezbkTools/internal/app"
	"ezbkTools/internal/domain"
)

// User — користувачі застосунку (таблиця users).
type User struct {
	db *sql.DB
}

func NewUser(db *sql.DB) *User {
	return &User{db: db}
}

// userColumns — порядок колонок, який очікує scanUser.
const userColumns = `id, login, password_hash, must_change_password, session_version,
	webhook_token, ezb_token, mono_webhook_registered_at`

func scanUser(row interface{ Scan(dest ...any) error }) (*domain.User, error) {
	var (
		user         domain.User
		mustChange   int
		registeredAt int64
	)
	if err := row.Scan(
		&user.ID,
		&user.Login,
		&user.PasswordHash,
		&mustChange,
		&user.SessionVersion,
		&user.WebhookToken,
		&user.EzbToken,
		&registeredAt,
	); err != nil {
		return nil, err
	}
	user.MustChangePassword = mustChange == 1
	if registeredAt != 0 {
		user.MonoWebhookRegisteredAt = time.Unix(registeredAt, 0)
	}
	return &user, nil
}

// oneBy — користувач за значенням унікальної колонки; column — лише константа з цього файлу.
func (u *User) oneBy(ctx context.Context, column string, value any) (*domain.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE ` + column + ` = ?`
	user, err := scanUser(u.db.QueryRowContext(ctx, query, value))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("read user by %s: %w", column, err)
	}
	return user, nil
}

func (u *User) ByID(ctx context.Context, id int64) (*domain.User, error) {
	return u.oneBy(ctx, "id", id)
}

func (u *User) ByLogin(ctx context.Context, login string) (*domain.User, error) {
	return u.oneBy(ctx, "login", login)
}

func (u *User) ByWebhookToken(ctx context.Context, token string) (*domain.User, error) {
	return u.oneBy(ctx, "webhook_token", token)
}

func (u *User) Create(ctx context.Context, user domain.User) (*domain.User, error) {
	mustChange := 0
	if user.MustChangePassword {
		mustChange = 1
	}
	query := `INSERT INTO users (login, password_hash, must_change_password, webhook_token)
			  VALUES (?, ?, ?, ?)
			  RETURNING ` + userColumns
	created, err := scanUser(u.db.QueryRowContext(ctx, query, user.Login, user.PasswordHash, mustChange, user.WebhookToken))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: users.login") {
			return nil, fmt.Errorf("%w: %s", app.ErrLoginTaken, user.Login)
		}
		return nil, fmt.Errorf("create user %s: %w", user.Login, err)
	}
	return created, nil
}

// FillOwner — умова на порожній хеш у самому UPDATE: .env не має перезаписати пароль,
// який власник уже змінив через PWA.
func (u *User) FillOwner(ctx context.Context, login, passwordHash, ezbToken string) error {
	query := `UPDATE users
			  SET login = ?, password_hash = ?, ezb_token = ?, updated_at = CURRENT_TIMESTAMP
			  WHERE id = ? AND password_hash = ''`
	if _, err := u.db.ExecContext(ctx, query, login, passwordHash, ezbToken, domain.OwnerUserID); err != nil {
		return fmt.Errorf("fill owner: %w", err)
	}
	return nil
}

func (u *User) SetPassword(ctx context.Context, id int64, passwordHash string) (int64, error) {
	query := `UPDATE users
			  SET password_hash = ?, must_change_password = 0,
			      session_version = session_version + 1, updated_at = CURRENT_TIMESTAMP
			  WHERE id = ?
			  RETURNING session_version`
	var version int64
	if err := u.db.QueryRowContext(ctx, query, passwordHash, id).Scan(&version); err != nil {
		return 0, fmt.Errorf("set password for user %d: %w", id, err)
	}
	return version, nil
}

func (u *User) SetEzbToken(ctx context.Context, id int64, token string) error {
	query := `UPDATE users SET ezb_token = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	if _, err := u.db.ExecContext(ctx, query, token, id); err != nil {
		return fmt.Errorf("set ezb token for user %d: %w", id, err)
	}
	return nil
}

func (u *User) SetMonoWebhookRegistered(ctx context.Context, id int64, at time.Time) error {
	query := `UPDATE users SET mono_webhook_registered_at = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	if _, err := u.db.ExecContext(ctx, query, at.Unix(), id); err != nil {
		return fmt.Errorf("set webhook registered for user %d: %w", id, err)
	}
	return nil
}
