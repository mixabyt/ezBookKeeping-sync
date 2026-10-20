package httpapi

import (
	"ezbkTools/internal/domain"
	"ezbkTools/internal/platform/logctx"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
	"time"
)

func (s *Server) monoWebHook() gin.HandlerFunc {
	return func(c *gin.Context) {
		req := StatementItemDTO{}
		if err := c.ShouldBindJSON(&req); err != nil {
			logctx.From(c.Request.Context()).Warn("invalid monobank webhook json", "error", err)
			s.respondError(c, err)
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

		err := s.syncService.Enqueue(c.Request.Context(), tx)
		if err != nil {
			logctx.From(c.Request.Context()).Error("Помилка при додаванні черги", zap.Error(err))
		}
		c.Status(http.StatusOK)

	}
}
func (s *Server) registerWebHook() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Status(http.StatusOK)
	}
}
