package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"ezbkTools/internal/domain"
	"ezbkTools/internal/platform/logctx"
	"golang.org/x/crypto/bcrypt"
)

const (
	minPasswordChars = 8
	// maxPasswordBytes — стеля bcrypt: довший пароль він відмовляється хешувати.
	maxPasswordBytes = 72
	maxLoginChars    = 64
	// tempPasswordChars — 16 символів base32, 80 біт: досить для пароля, який живе до першого входу.
	tempPasswordChars = 16
)

var (
	// ErrInvalidCredentials — неправильний логін або пароль.
	ErrInvalidCredentials = errors.New("неправильний логін або пароль")
	// ErrBrokenPasswordHash — у БД лежить не bcrypt-хеш, це збій конфігурації, а не невірний пароль.
	ErrBrokenPasswordHash = errors.New("хеш пароля некоректний")
	// ErrWrongCurrentPassword — окремо від ErrInvalidCredentials: людина вже увійшла,
	// і така помилка не має виглядати для PWA як завершена сесія.
	ErrWrongCurrentPassword = errors.New("поточний пароль неправильний")
	ErrWeakPassword         = fmt.Errorf("пароль має містити щонайменше %d символів і не більше %d байт", minPasswordChars, maxPasswordBytes)
	ErrSamePassword         = errors.New("новий пароль має відрізнятися від поточного")

	ErrInvalidLogin = fmt.Errorf("логін має бути непорожнім, без пробілів, до %d символів", maxLoginChars)
	ErrLoginTaken   = errors.New("такий логін уже зайнятий")
	// ErrOwnerNotConfigured — рядок власника ще порожній, а в .env немає даних, щоб його заповнити.
	ErrOwnerNotConfigured = errors.New("власник не налаштований: задайте AUTH_LOGIN і AUTH_PASSWORD_HASH")

	// ErrNoEzbToken — не збій, а «чекаємо людину», як і відсутній мапінг.
	ErrNoEzbToken = errors.New("не задано токен ezBookkeeping")
	ErrEmptyToken = errors.New("вставте токен")
	// ErrBadEzbToken — ezBookkeeping не прийняв токен під час пробного запиту.
	ErrBadEzbToken = errors.New("ezBookkeeping не прийняв цей токен")

	ErrBadMonoToken    = errors.New("Monobank не прийняв цей ключ")
	ErrMonoRateLimited = errors.New("Monobank обмежив кількість запитів, спробуйте за хвилину")
	// ErrMonoRejected — ключ правильний, але банк не зміг підтвердити адресу вебхука.
	ErrMonoRejected = errors.New("Monobank не прийняв адресу вебхука")
	// ErrWebhookNotConfigured — на сервері не задано MONO_WEBHOOK_BASE_URL.
	ErrWebhookNotConfigured = errors.New("на сервері не задано MONO_WEBHOOK_BASE_URL")
	// ErrUpstreamUnavailable — зовнішній сервіс не відповів; текст безпечно показувати людині.
	ErrUpstreamUnavailable = errors.New("зовнішній сервіс не відповідає, спробуйте пізніше")
)

// UserService — облікові записи: вхід, зміна пароля, особисті налаштування.
type UserService struct {
	users          UserRepository
	ledgers        LedgerFactory
	registrar      MonoWebhookRegistrar
	waker          JobWaker
	webhookBaseURL string
}

func NewUserService(
	users UserRepository,
	ledgers LedgerFactory,
	registrar MonoWebhookRegistrar,
	waker JobWaker,
	webhookBaseURL string,
) *UserService {
	return &UserService{
		users:          users,
		ledgers:        ledgers,
		registrar:      registrar,
		waker:          waker,
		webhookBaseURL: strings.TrimRight(webhookBaseURL, "/"),
	}
}

// dummyPasswordHash — проти якого хешу звіряти пароль, коли логіна немає.
// Рахується один раз і лише тоді, коли знадобиться: bcrypt повільний навмисно.
var dummyPasswordHash = sync.OnceValue(func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte(rand.Text()), bcrypt.DefaultCost)
	if err != nil {
		panic(fmt.Sprintf("bcrypt dummy hash: %v", err))
	}
	return hash
})

// Authenticate перевіряє логін і пароль. Неіснуючий логін теж проганяє bcrypt,
// інакше за часом відповіді можна було б визначити, які логіни існують.
func (s *UserService) Authenticate(ctx context.Context, login, password string) (*domain.User, error) {
	user, err := s.users.ByLogin(ctx, login)
	if err != nil {
		return nil, err
	}
	if user == nil || user.PasswordHash == "" {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash(), []byte(password))
		return nil, ErrInvalidCredentials
	}

	switch err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); {
	case err == nil:
		return user, nil
	case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
		return nil, ErrInvalidCredentials
	default:
		return nil, fmt.Errorf("%w: %w", ErrBrokenPasswordHash, err)
	}
}

func (s *UserService) ByID(ctx context.Context, id int64) (*domain.User, error) {
	return s.users.ByID(ctx, id)
}

func (s *UserService) ByWebhookToken(ctx context.Context, token string) (*domain.User, error) {
	return s.users.ByWebhookToken(ctx, token)
}

// ChangePassword міняє пароль і повертає користувача з новою версією сесії:
// усі попередні сесії після цього недійсні.
func (s *UserService) ChangePassword(ctx context.Context, userID int64, current, next string) (*domain.User, error) {
	user, err := s.users.ByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user %d not found", userID)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(current)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return nil, ErrWrongCurrentPassword
		}
		return nil, fmt.Errorf("%w: %w", ErrBrokenPasswordHash, err)
	}
	if utf8.RuneCountInString(next) < minPasswordChars || len(next) > maxPasswordBytes {
		return nil, ErrWeakPassword
	}
	if current == next {
		return nil, ErrSamePassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	version, err := s.users.SetPassword(ctx, userID, string(hash))
	if err != nil {
		return nil, err
	}

	user.PasswordHash = string(hash)
	user.MustChangePassword = false
	user.SessionVersion = version
	return user, nil
}

// SetEzbToken перевіряє токен пробним запитом і лише тоді зберігає його.
// Воркер прокидається: транзакції могли чекати саме на токен.
func (s *UserService) SetEzbToken(ctx context.Context, userID int64, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrEmptyToken
	}

	if _, err := s.ledgers(token).Accounts(ctx); err != nil {
		if errors.Is(err, ErrBadEzbToken) {
			return ErrBadEzbToken
		}
		logctx.From(ctx).Error("не вдалося перевірити токен ezBookkeeping", "user", userID, "err", err)
		return ErrUpstreamUnavailable
	}

	if err := s.users.SetEzbToken(ctx, userID, token); err != nil {
		return err
	}
	s.waker.Wake()
	return nil
}

// RegisterMonoWebhook вказує банку адресу вебхука цього користувача.
// Ключ Monobank навмисно нікуди не зберігається: він потрібен лише для цього виклику,
// а дає доступ до виписки.
func (s *UserService) RegisterMonoWebhook(ctx context.Context, userID int64, monoToken string) (time.Time, error) {
	monoToken = strings.TrimSpace(monoToken)
	if monoToken == "" {
		return time.Time{}, ErrEmptyToken
	}
	if s.webhookBaseURL == "" {
		return time.Time{}, ErrWebhookNotConfigured
	}

	user, err := s.users.ByID(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}
	if user == nil {
		return time.Time{}, fmt.Errorf("user %d not found", userID)
	}

	if err := s.registrar.SetWebhook(ctx, monoToken, s.webhookBaseURL+"/"+user.WebhookToken); err != nil {
		return time.Time{}, err
	}

	now := time.Now()
	if err := s.users.SetMonoWebhookRegistered(ctx, userID, now); err != nil {
		return time.Time{}, err
	}
	return now, nil
}

// BootstrapOwner заповнює рядок власника даними з .env при першому старті після міграції.
// Далі джерело правди — БД: зміна пароля в PWA не має відкочуватись після рестарту.
func BootstrapOwner(ctx context.Context, users UserRepository, login, passwordHash, ezbToken string) error {
	owner, err := users.ByID(ctx, domain.OwnerUserID)
	if err != nil {
		return err
	}
	if owner == nil {
		return fmt.Errorf("у БД немає власника (user %d)", domain.OwnerUserID)
	}
	if owner.PasswordHash != "" {
		return nil
	}

	if login == "" || passwordHash == "" {
		return ErrOwnerNotConfigured
	}
	// битий хеш краще побачити зараз, а не при першому вході
	if _, err := bcrypt.Cost([]byte(passwordHash)); err != nil {
		return fmt.Errorf("%w: перевірте, що AUTH_PASSWORD_HASH в .env узятий в одинарні лапки: %w", ErrBrokenPasswordHash, err)
	}
	return users.FillOwner(ctx, login, passwordHash, ezbToken)
}

// CreateUser створює користувача з випадковим тимчасовим паролем і токеном вебхука.
// Тимчасовий пароль повертається один раз; у БД лежить лише його хеш.
func CreateUser(ctx context.Context, users UserRepository, login string) (*domain.User, string, error) {
	if !validLogin(login) {
		return nil, "", ErrInvalidLogin
	}

	// власник заповнюється при старті сервісу; якби логін зайняли раніше, той старт упав би на UNIQUE
	owner, err := users.ByID(ctx, domain.OwnerUserID)
	if err != nil {
		return nil, "", err
	}
	if owner == nil || owner.PasswordHash == "" {
		return nil, "", errors.New("власник ще не заповнений: спершу запустіть сервіс")
	}

	password := rand.Text()[:tempPasswordChars]
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("hash password: %w", err)
	}

	created, err := users.Create(ctx, domain.User{
		Login:              login,
		PasswordHash:       string(hash),
		MustChangePassword: true,
		WebhookToken:       newWebhookToken(),
	})
	if err != nil {
		return nil, "", err
	}
	return created, password, nil
}

func validLogin(login string) bool {
	if login == "" || utf8.RuneCountInString(login) > maxLoginChars {
		return false
	}
	return strings.IndexFunc(login, unicode.IsSpace) == -1
}

// newWebhookToken — 256 випадкових біт: адресу вебхука не має бути можливо вгадати.
func newWebhookToken() string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw) // crypto/rand.Read не повертає помилок
	return hex.EncodeToString(raw)
}

// ledgerForUser — клієнт ezBookkeeping для користувача; токена ще немає — ErrNoEzbToken.
func ledgerForUser(ctx context.Context, userID int64, users UserRepository, ledgers LedgerFactory) (Ledger, error) {
	user, err := users.ByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user %d not found", userID)
	}
	if user.EzbToken == "" {
		return nil, ErrNoEzbToken
	}
	return ledgers(user.EzbToken), nil
}
