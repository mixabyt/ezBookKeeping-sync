package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ezbkTools/internal/app"
	"ezbkTools/internal/domain"
	"ezbkTools/internal/platform/logctx"
	"github.com/gin-gonic/gin"
)

// AuthConfig — секрет для підпису токена сесії. Користувачі й паролі живуть у БД.
type AuthConfig struct {
	Secret string
	TTL    time.Duration
}

const (
	sessionCookie = "ezbk_session"
	defaultTTL    = 30 * 24 * time.Hour
	// currentUserKey — під цим ключем requireAuth кладе користувача в gin.Context.
	currentUserKey = "current_user"
)

var (
	// ErrUnauthorized — немає токена, він прострочений, підпис не збігається або сесію відкликано.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrAuthNotConfigured — у env немає секрету або в БД битий хеш пароля, вхід вимкнено.
	ErrAuthNotConfigured = errors.New("авторизація не налаштована")
	// ErrTooManyAttempts — забагато невдалих спроб входу з цього IP.
	ErrTooManyAttempts = errors.New("забагато спроб входу, спробуйте пізніше")
	// ErrPasswordChangeRequired — вхід із тимчасовим паролем: спершу треба задати свій.
	ErrPasswordChangeRequired = errors.New("спершу змініть тимчасовий пароль")
)

func (c AuthConfig) configured() bool {
	return c.Secret != ""
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

		user, err := s.users.Authenticate(c.Request.Context(), req.Login, req.Password)
		switch {
		case errors.Is(err, app.ErrInvalidCredentials):
			s.failedLogin(c.ClientIP())
			logctx.From(c.Request.Context()).Warn("невдалий вхід", "ip", c.ClientIP(), "login", req.Login)
			s.errorResponse(c, http.StatusUnauthorized, ErrUnauthorized)
			return
		case errors.Is(err, app.ErrBrokenPasswordHash):
			// це не "невірний пароль", а поламані дані в БД
			logctx.From(c.Request.Context()).Error("хеш пароля в БД не схожий на bcrypt", "login", req.Login, "err", err)
			s.errorResponse(c, http.StatusInternalServerError, ErrAuthNotConfigured)
			return
		case err != nil:
			s.respondError(c, err)
			return
		}

		token, expires := issueToken(user.ID, user.SessionVersion, s.auth.Secret, s.auth.ttl())
		s.setSessionCookie(c, token, expires)
		response := userToDTO(user)
		response.Token = token // для PWA, якщо зручніше тримати в Authorization
		response.ExpiresAt = expires.Unix()
		s.respondOK(c, response)
	}
}

// logout гасить кукі на цьому пристрої. Сесії на інших пристроях живуть до
// кінця TTL або до зміни пароля, яка піднімає версію сесії.
func (s *Server) logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		s.setSessionCookie(c, "", time.Unix(0, 0))
		s.respondNoContent(c)
	}
}

// requireAuth пропускає далі лише з валідним токеном. Користувач читається з БД
// на кожен запит: так зміна пароля одразу гасить старі сесії, а SQLite локальна і дешева.
func (s *Server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.auth.configured() {
			s.errorResponse(c, http.StatusInternalServerError, ErrAuthNotConfigured)
			c.Abort()
			return
		}

		userID, version, err := parseToken(tokenFromRequest(c), s.auth.Secret)
		if err != nil {
			s.errorResponse(c, http.StatusUnauthorized, ErrUnauthorized)
			c.Abort()
			return
		}

		user, err := s.users.ByID(c.Request.Context(), userID)
		if err != nil {
			s.respondError(c, err)
			c.Abort()
			return
		}
		if user == nil || user.SessionVersion != version {
			s.errorResponse(c, http.StatusUnauthorized, ErrUnauthorized)
			c.Abort()
			return
		}

		c.Set(currentUserKey, user)
		c.Next()
	}
}

// requirePasswordChanged закриває API, поки людина не змінить тимчасовий пароль.
// На сервері, а не лише вікном у PWA: інакше тимчасовий пароль лишався б робочим назавжди.
func (s *Server) requirePasswordChanged() gin.HandlerFunc {
	return func(c *gin.Context) {
		if currentUser(c).MustChangePassword {
			s.respondError(c, ErrPasswordChangeRequired)
			c.Abort()
			return
		}
		c.Next()
	}
}

// currentUser — користувач, якого поклав requireAuth.
func currentUser(c *gin.Context) *domain.User {
	return c.MustGet(currentUserKey).(*domain.User)
}

func currentUserID(c *gin.Context) int64 {
	return currentUser(c).ID
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

// Формат токена: base64url("<userID>|<версія сесії>|<exp>") + "." + base64url(HMAC-SHA256 від payload).
// Підпис гарантує, що payload не підміняли; секрет знає лише сервер.
func issueToken(userID, version int64, secret string, ttl time.Duration) (string, time.Time) {
	expires := time.Now().Add(ttl)
	payload := strconv.FormatInt(userID, 10) + "|" + strconv.FormatInt(version, 10) + "|" + strconv.FormatInt(expires.Unix(), 10)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return encoded + "." + sign(encoded, secret), expires
}

func parseToken(token, secret string) (userID, version int64, err error) {
	encoded, signature, found := strings.Cut(token, ".")
	if !found || encoded == "" {
		return 0, 0, ErrUnauthorized
	}
	if !hmac.Equal([]byte(signature), []byte(sign(encoded, secret))) {
		return 0, 0, ErrUnauthorized
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return 0, 0, ErrUnauthorized
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return 0, 0, ErrUnauthorized // зокрема токени старого формату "<login>|<exp>"
	}

	userID, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, ErrUnauthorized
	}
	version, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, ErrUnauthorized
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || time.Now().After(time.Unix(exp, 0)) {
		return 0, 0, fmt.Errorf("%w: токен прострочено", ErrUnauthorized)
	}
	return userID, version, nil
}

func sign(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
