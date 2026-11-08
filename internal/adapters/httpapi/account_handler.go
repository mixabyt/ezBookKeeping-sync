package httpapi

import (
	"errors"
	"net/http"
	"time"

	"ezbkTools/internal/app"
	"ezbkTools/internal/domain"
	"ezbkTools/internal/platform/logctx"
	"github.com/gin-gonic/gin"
)

// UserDTO — стан облікового запису для PWA. Самих секретів тут немає навмисно:
// лише ознаки, чи вони задані.
type UserDTO struct {
	Login              string `json:"login"`
	MustChangePassword bool   `json:"must_change_password"`
	EzbConfigured      bool   `json:"ezb_configured"`
	// MonoWebhookRegisteredAt порожній — вебхук через PWA ще не реєстрували.
	MonoWebhookRegisteredAt string `json:"mono_webhook_registered_at"`

	// Лише у відповіді на вхід.
	Token     string `json:"token,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

func userToDTO(user *domain.User) UserDTO {
	registeredAt := ""
	if !user.MonoWebhookRegisteredAt.IsZero() {
		registeredAt = user.MonoWebhookRegisteredAt.Format(time.RFC3339)
	}
	return UserDTO{
		Login:                   user.Login,
		MustChangePassword:      user.MustChangePassword,
		EzbConfigured:           user.EzbToken != "",
		MonoWebhookRegisteredAt: registeredAt,
	}
}

// me — GET /api/me: чи є сесія і що людині лишилось налаштувати.
func (s *Server) me() gin.HandlerFunc {
	return func(c *gin.Context) {
		s.respondOK(c, userToDTO(currentUser(c)))
	}
}

// ChangePasswordRequest — тіло POST /api/change-password. Без binding-тегів навмисно:
// порожні значення перевіряє app, і людина бачить українське повідомлення.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// changePassword — POST /api/change-password. Працює і під час примусової зміни.
// Старі сесії гаснуть, тож цій людині одразу видаємо нову кукі.
func (s *Server) changePassword() gin.HandlerFunc {
	return func(c *gin.Context) {
		// той самий лічильник, що й на вході: украдена сесія не має давати перебір пароля
		if !s.loginAllowed(c.ClientIP()) {
			s.errorResponse(c, http.StatusTooManyRequests, ErrTooManyAttempts)
			return
		}

		var req ChangePasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.respondError(c, err)
			return
		}

		user, err := s.users.ChangePassword(c.Request.Context(), currentUserID(c), req.CurrentPassword, req.NewPassword)
		if err != nil {
			if errors.Is(err, app.ErrWrongCurrentPassword) {
				s.failedLogin(c.ClientIP())
			}
			logctx.From(c.Request.Context()).Warn("не вдалося змінити пароль", "user", currentUserID(c), "err", err)
			s.respondError(c, err)
			return
		}

		token, expires := issueToken(user.ID, user.SessionVersion, s.auth.Secret, s.auth.ttl())
		s.setSessionCookie(c, token, expires)
		logctx.From(c.Request.Context()).Info("пароль змінено", "user", user.ID)
		s.respondOK(c, userToDTO(user))
	}
}

// TokenRequest — тіло запитів, що приймають токен сервісу.
type TokenRequest struct {
	Token string `json:"token"`
}

// putEzbToken — PUT /api/ezb-token: токен перевіряється пробним запитом і лише тоді зберігається.
func (s *Server) putEzbToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req TokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.respondError(c, err)
			return
		}

		if err := s.users.SetEzbToken(c.Request.Context(), currentUserID(c), req.Token); err != nil {
			logctx.From(c.Request.Context()).Warn("не вдалося зберегти токен ezBookkeeping", "user", currentUserID(c), "err", err)
			s.respondError(c, err)
			return
		}
		logctx.From(c.Request.Context()).Info("збережено токен ezBookkeeping", "user", currentUserID(c))
		s.respondOK(c, gin.H{"ezb_configured": true})
	}
}

// registerMonoWebhook — POST /api/mono/webhook: реєструє в Monobank вебхук цього користувача.
// Ключ використовується один раз і не зберігається.
func (s *Server) registerMonoWebhook() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req TokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.respondError(c, err)
			return
		}

		registeredAt, err := s.users.RegisterMonoWebhook(c.Request.Context(), currentUserID(c), req.Token)
		if err != nil {
			logctx.From(c.Request.Context()).Warn("не вдалося зареєструвати вебхук", "user", currentUserID(c), "err", err)
			s.respondError(c, err)
			return
		}
		logctx.From(c.Request.Context()).Info("вебхук зареєстровано", "user", currentUserID(c))
		s.respondOK(c, gin.H{"mono_webhook_registered_at": registeredAt.Format(time.RFC3339)})
	}
}
