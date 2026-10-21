package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ezbkTools/internal/platform/logctx"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// AuthConfig — єдиний користувач із env: логін, bcrypt-хеш пароля і секрет для підпису токена.
// Хеш генерується один раз командою cmd/hashpw; сам пароль ніде не зберігається.
type AuthConfig struct {
	Login        string
	PasswordHash string
	Secret       string
	TTL          time.Duration
}

const (
	sessionCookie = "ezbk_session"
	defaultTTL    = 30 * 24 * time.Hour
)

var (
	// ErrUnauthorized — немає токена, він прострочений або підпис не збігається.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrAuthNotConfigured — у env немає логіна/хеша/секрету, вхід вимкнено.
	ErrAuthNotConfigured = errors.New("авторизація не налаштована")
	// ErrTooManyAttempts — забагато невдалих спроб входу з цього IP.
	ErrTooManyAttempts = errors.New("забагато спроб входу, спробуйте пізніше")
)

func (c AuthConfig) configured() bool {
	return c.Login != "" && c.PasswordHash != "" && c.Secret != ""
}

func (c AuthConfig) ttl() time.Duration {
	if c.TTL <= 0 {
		return defaultTTL
	}
	return c.TTL
}

type loginRequest struct {
	Login    string `json:"login" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// login перевіряє пару логін/пароль і видає токен у HttpOnly-кукі.
func (s *Server) login() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.auth.configured() {
			s.errorResponse(c, http.StatusInternalServerError, ErrAuthNotConfigured)
			return
		}
		if !s.loginAllowed(c.ClientIP()) {
			s.errorResponse(c, http.StatusTooManyRequests, ErrTooManyAttempts)
			return
		}

		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.respondError(c, err)
			return
		}

		// логін порівнюємо в константний час, щоб не зливати його побайтово,
		// пароль — bcrypt-ом, він повільний навмисно
		loginOK := subtle.ConstantTimeCompare([]byte(req.Login), []byte(s.auth.Login)) == 1
		passErr := bcrypt.CompareHashAndPassword([]byte(s.auth.PasswordHash), []byte(req.Password))

		// помилка формату — це не "невірний пароль", а поламаний конфіг:
		// найчастіше AUTH_PASSWORD_HASH записали без лапок і godotenv зʼїв $...
		if isBrokenHash(passErr) {
			logctx.From(c.Request.Context()).Error(
				"AUTH_PASSWORD_HASH не схожий на bcrypt-хеш — перевірте, що значення в .env узяте в одинарні лапки",
				"err", passErr)
			s.errorResponse(c, http.StatusInternalServerError, ErrAuthNotConfigured)
			return
		}

		if !loginOK || passErr != nil {
			s.failedLogin(c.ClientIP())
			logctx.From(c.Request.Context()).Warn("невдалий вхід", "ip", c.ClientIP(), "login", req.Login)
			s.errorResponse(c, http.StatusUnauthorized, ErrUnauthorized)
			return
		}

		token, expires := issueToken(s.auth.Login, s.auth.Secret, s.auth.ttl())
		s.setSessionCookie(c, token, expires)
		s.respondOK(c, gin.H{
			"login":      s.auth.Login,
			"token":      token, // для PWA, якщо зручніше тримати в Authorization
			"expires_at": expires.Unix(),
		})
	}
}

// logout гасить кукі. Токен без стану, тож відкликати його на сервері нічим —
// саме тому TTL обмежений, а секрет можна змінити, щоб інвалідувати всі токени.
func (s *Server) logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		s.setSessionCookie(c, "", time.Unix(0, 0))
		s.respondNoContent(c)
	}
}

// requireAuth пропускає далі лише з валідним токеном.
func (s *Server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.auth.configured() {
			s.errorResponse(c, http.StatusInternalServerError, ErrAuthNotConfigured)
			c.Abort()
			return
		}

		token := tokenFromRequest(c)
		login, err := parseToken(token, s.auth.Secret)
		if err != nil {
			s.errorResponse(c, http.StatusUnauthorized, ErrUnauthorized)
			c.Abort()
			return
		}

		c.Set("login", login)
		c.Next()
	}
}

// isBrokenHash — помилки, які означають "хеш у конфізі некоректний",
// а не "пароль не підійшов".
func isBrokenHash(err error) bool {
	if err == nil {
		return false
	}
	var prefixErr bcrypt.InvalidHashPrefixError
	var costErr bcrypt.InvalidCostError
	return errors.Is(err, bcrypt.ErrHashTooShort) ||
		errors.As(err, &prefixErr) ||
		errors.As(err, &costErr)
}

func (s *Server) setSessionCookie(c *gin.Context, token string, expires time.Time) {
	// Secure ставимо, коли запит прийшов по HTTPS (за проксі — за X-Forwarded-Proto),
	// інакше кукі не працюватиме при локальній розробці по http
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func tokenFromRequest(c *gin.Context) string {
	if cookie, err := c.Request.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

// Формат токена: base64url("<login>|<exp>") + "." + base64url(HMAC-SHA256 від payload).
// Підпис гарантує, що payload не підміняли; секрет знає лише сервер.
func issueToken(login, secret string, ttl time.Duration) (string, time.Time) {
	expires := time.Now().Add(ttl)
	payload := login + "|" + strconv.FormatInt(expires.Unix(), 10)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return encoded + "." + sign(encoded, secret), expires
}

func parseToken(token, secret string) (string, error) {
	encoded, signature, found := strings.Cut(token, ".")
	if !found || encoded == "" {
		return "", ErrUnauthorized
	}
	if !hmac.Equal([]byte(signature), []byte(sign(encoded, secret))) {
		return "", ErrUnauthorized
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrUnauthorized
	}
	login, expStr, found := strings.Cut(string(raw), "|")
	if !found {
		return "", ErrUnauthorized
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().After(time.Unix(exp, 0)) {
		return "", fmt.Errorf("%w: токен прострочено", ErrUnauthorized)
	}
	return login, nil
}

func sign(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
