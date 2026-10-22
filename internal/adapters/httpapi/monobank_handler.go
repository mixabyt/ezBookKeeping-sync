package httpapi

import (
	"context"
	"ezbkTools/internal/domain"
	"ezbkTools/internal/platform/logctx"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) checkMonoSign() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// ok, err := monobank.CheckSign(ctx.Request)
		// if err != nil {
		// 	logctx.From(ctx).Error("Error with sign", "err", err)
		// 	ctx.AbortWithStatus(http.StatusOK)
		// 	return
		// }
		// if !ok {
		// 	ctx.AbortWithStatus(http.StatusForbidden)
		// 	return
		// }
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
		item := req.Data.StatementItem
		tx := &domain.BankTransaction{
			Source:      domain.SourceMono,
			ID:          domain.MonoPrefix + item.ID,
			AccountID:   req.Data.Account,
			Time:        time.Unix(item.Time, 0),
			Amount:      int64(item.Amount),
			MCC:         item.MCC,
			Description: item.Description,
			Hold:        item.Hold,
		}
		ctxTimeout, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		err := s.syncService.Enqueue(ctxTimeout, tx)
		if err != nil {
			logctx.From(c.Request.Context()).Error("Помилка при додаванні черги", "tx", tx.ID, "err", err)
		}
		c.Status(http.StatusOK)

	}
}
func (s *Server) registerWebHook() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Status(http.StatusOK)
	}
}
