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
	"time"

	"github.com/gin-gonic/gin"
)

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

		err := s.syncService.Enqueue(ctxTimeout, tx)
		if err != nil {
			logctx.From(c.Request.Context()).Error("Помилка при додаванні черги", "tx", tx.ID, "err", err)
		}
		c.Status(http.StatusOK)

	}
}

// ErrInvalidWebhook — збережений вебхук не можна обробити ніколи (підробка, зламане тіло):
// повтор нічого не змінить, тож його можна викидати з черги.
var ErrInvalidWebhook = errors.New("invalid monobank webhook")

// ReplayMonoWebhook обробляє вебхук, який nginx зберіг, поки сервер не відповідав.
// Підпис перевіряється на body як є і до парсингу — як у checkMonoSign для живого запиту.
//
// Помилки без ErrInvalidWebhook — тимчасові: банк не віддав ключ, не той X-Key-Id
// (після ротації), впала БД. Такий вебхук варто спробувати пізніше, а не викидати.
func (s *Server) ReplayMonoWebhook(ctx context.Context, header http.Header, body []byte) error {
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
	return s.syncService.Enqueue(ctx, monoToBankTransaction(req))
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
