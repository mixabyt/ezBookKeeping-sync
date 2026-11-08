package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"ezbkTools/internal/adapters/monobank"
	"ezbkTools/internal/domain"
	"ezbkTools/internal/platform/logctx"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// monoWebhookPath — шлях вебхука; нижче нього йде токен користувача.
	monoWebhookPath = "/mono-webhook"
	// webhookUserIDKey — під цим ключем middleware кладе власника вебхука в gin.Context.
	webhookUserIDKey = "webhook_user_id"
)

// webhookUserByToken визначає, чий це вебхук. Невідомий токен — 404: для такої адреси
// банку й справді немає отримувача. Збій БД — 500: тоді банк повторить запит, і транзакція не загубиться.
func (s *Server) webhookUserByToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := s.users.ByWebhookToken(c.Request.Context(), c.Param("token"))
		if err != nil {
			logctx.From(c.Request.Context()).Error("не вдалося знайти власника вебхука", "err", err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		if user == nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Set(webhookUserIDKey, user.ID)
		c.Next()
	}
}

// webhookOwner — для старої адреси без токена.
func (s *Server) webhookOwner() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(webhookUserIDKey, domain.OwnerUserID)
		c.Next()
	}
}

func (s *Server) checkMonoSign() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ok, err := monobank.CheckSign(ctx.Request)
		if err != nil {
			logctx.From(ctx).Error("Error with sign", "err", err)
			ctx.AbortWithStatus(http.StatusOK)
			return
		}
		if !ok {
			ctx.AbortWithStatus(http.StatusForbidden)
			return
		}
		ctx.Next()
	}
}

func (s *Server) monoWebHook() gin.HandlerFunc {
	return func(c *gin.Context) {
		req := StatementItemDTO{}
		if err := c.ShouldBindJSON(&req); err != nil {
			logctx.From(c.Request.Context()).Warn("invalid monobank webhook json", "error", err)
			c.Status(http.StatusOK)
			return
		}
		tx := monoToBankTransaction(req)
		ctxTimeout, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		err := s.syncService.Enqueue(ctxTimeout, c.GetInt64(webhookUserIDKey), tx)
		if err != nil {
			logctx.From(c.Request.Context()).Error("Помилка при додаванні черги", "tx", tx.ID, "err", err)
		}
		c.Status(http.StatusOK)

	}
}

// ErrInvalidWebhook — збережений вебхук не можна обробити ніколи (підробка, зламане тіло):
// повтор нічого не змінить, тож його можна викидати з черги.
var ErrInvalidWebhook = errors.New("invalid monobank webhook")

// ParseMonoWebhookURI розбирає шлях збереженого запиту на токен користувача.
// ok == false — це не вебхук Monobank. Порожній токен — стара адреса без токена (власник).
func ParseMonoWebhookURI(uri string) (token string, ok bool) {
	path, _, _ := strings.Cut(uri, "?")
	if path == monoWebhookPath {
		return "", true
	}
	token, found := strings.CutPrefix(path, monoWebhookPath+"/")
	if !found || token == "" || strings.Contains(token, "/") {
		return "", false
	}
	return token, true
}

// ReplayMonoWebhook обробляє вебхук, який nginx зберіг, поки сервер не відповідав.
// webhookToken — із шляху запиту (ParseMonoWebhookURI), порожній — власник.
// Підпис перевіряється на body як є і до парсингу — як у checkMonoSign для живого запиту.
//
// Помилки без ErrInvalidWebhook — тимчасові: банк не віддав ключ, не той X-Key-Id
// (після ротації), впала БД. Такий вебхук варто спробувати пізніше, а не викидати.
func (s *Server) ReplayMonoWebhook(ctx context.Context, webhookToken string, header http.Header, body []byte) error {
	userID := domain.OwnerUserID
	if webhookToken != "" {
		user, err := s.users.ByWebhookToken(ctx, webhookToken)
		if err != nil {
			return fmt.Errorf("find webhook user: %w", err)
		}
		if user == nil {
			return fmt.Errorf("%w: невідомий токен вебхука", ErrInvalidWebhook)
		}
		userID = user.ID
	}

	ok, err := monobank.Verify(ctx, header, body)
	switch {
	case errors.Is(err, monobank.ErrNoSignature), errors.Is(err, monobank.ErrBadSignatureFormat):
		return fmt.Errorf("%w: %w", ErrInvalidWebhook, err)
	case err != nil:
		return fmt.Errorf("verify sign: %w", err)
	case !ok:
		return fmt.Errorf("%w: підпис не зійшовся", ErrInvalidWebhook)
	}

	var req StatementItemDTO
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidWebhook, err)
	}
	if req.Data.StatementItem.ID == "" {
		return fmt.Errorf("%w: порожній id транзакції (type %q)", ErrInvalidWebhook, req.Type)
	}
	return s.syncService.Enqueue(ctx, userID, monoToBankTransaction(req))
}

func monoToBankTransaction(req StatementItemDTO) *domain.BankTransaction {
	item := req.Data.StatementItem
	return &domain.BankTransaction{
		Source:      domain.SourceMono,
		ID:          domain.MonoPrefix + item.ID,
		AccountID:   req.Data.Account,
		Time:        time.Unix(item.Time, 0),
		Amount:      int64(item.Amount),
		MCC:         item.MCC,
		Description: item.Description,
		Hold:        item.Hold,
	}
}

func (s *Server) registerWebHook() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Status(http.StatusOK)
	}
}
