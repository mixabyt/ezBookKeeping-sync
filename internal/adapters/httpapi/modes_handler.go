package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"ezbkTools/internal/domain"
	"ezbkTools/internal/platform/logctx"
	"github.com/gin-gonic/gin"
)

// SettingsDTO — налаштування для сторінки «Налаштування».
type SettingsDTO struct {
	MappingMode string `json:"mapping_mode" binding:"required"` // 'mcc' | 'merchant' | 'manual'
}

// getSettings — GET /api/settings
func (s *Server) getSettings() gin.HandlerFunc {
	return func(c *gin.Context) {
		mode, err := s.mappingService.MappingMode(c.Request.Context())
		if err != nil {
			s.respondError(c, err)
			return
		}
		s.respondOK(c, SettingsDTO{MappingMode: string(mode)})
	}
}

// putSettings — PUT /api/settings: перемикання режиму діє й на джоби, що вже чекають.
func (s *Server) putSettings() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SettingsDTO
		if err := c.ShouldBindJSON(&req); err != nil {
			s.respondError(c, err)
			return
		}
		mode := domain.MappingMode(req.MappingMode)
		if err := s.mappingService.SetMappingMode(c.Request.Context(), mode); err != nil {
			s.respondError(c, err)
			return
		}
		logctx.From(c.Request.Context()).Info("змінено режим мапінгу", "mode", mode)
		s.respondOK(c, req)
	}
}

// WaitingMerchantDTO — продавець, на правило для якого чекають транзакції.
type WaitingMerchantDTO struct {
	MCC         int    `json:"mcc"`
	Direction   string `json:"direction"`
	Description string `json:"description"`
	WaitingJobs int    `json:"waiting_jobs"`
}

// waitingMerchants — GET /api/waiting-merchants (картки режиму merchant).
func (s *Server) waitingMerchants() gin.HandlerFunc {
	return func(c *gin.Context) {
		merchants, err := s.mappingService.WaitingMerchants(c.Request.Context())
		if err != nil {
			logctx.From(c.Request.Context()).Error("не вдалося отримати продавців, що чекають", "err", err)
			s.respondError(c, err)
			return
		}
		items := make([]WaitingMerchantDTO, 0, len(merchants))
		for _, m := range merchants {
			items = append(items, WaitingMerchantDTO{
				MCC: m.MCC, Direction: string(m.Direction), Description: m.Description, WaitingJobs: m.WaitingJobs,
			})
		}
		s.respondOK(c, gin.H{"items": items, "count": len(items)})
	}
}

// MerchantRuleDTO — правило продавця.
type MerchantRuleDTO struct {
	ID            int64  `json:"id"`
	MCC           int    `json:"mcc"`
	Direction     string `json:"direction"`
	Description   string `json:"description"`
	EzbCategoryID string `json:"ezb_category_id"`
	Enabled       bool   `json:"enabled"`
}

func merchantRuleToDTO(rule domain.MerchantRule) MerchantRuleDTO {
	return MerchantRuleDTO{
		ID:            rule.ID,
		MCC:           rule.MCC,
		Direction:     string(rule.Direction),
		Description:   rule.Description,
		EzbCategoryID: rule.EzbCategoryID,
		Enabled:       rule.Enabled,
	}
}

// listMerchantRules — GET /api/merchant-rules
func (s *Server) listMerchantRules() gin.HandlerFunc {
	return func(c *gin.Context) {
		rules, err := s.mappingService.MerchantRules(c.Request.Context())
		if err != nil {
			logctx.From(c.Request.Context()).Error("не вдалося отримати правила продавців", "err", err)
			s.respondError(c, err)
			return
		}
		items := make([]MerchantRuleDTO, 0, len(rules))
		for _, rule := range rules {
			items = append(items, merchantRuleToDTO(rule))
		}
		s.respondOK(c, gin.H{"items": items, "count": len(items)})
	}
}

// CreateMerchantRuleRequest — тіло POST /api/merchant-rules.
// ApplyToMCC — заразом правило на весь MCC («для всього MCC 5818»).
type CreateMerchantRuleRequest struct {
	MCC           int    `json:"mcc"`
	Direction     string `json:"direction" binding:"required"`
	Description   string `json:"description" binding:"required"`
	EzbCategoryID string `json:"ezb_category_id" binding:"required"`
	ApplyToMCC    bool   `json:"apply_to_mcc"`
}

// createMerchantRule — POST /api/merchant-rules
func (s *Server) createMerchantRule() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateMerchantRuleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.respondError(c, err)
			return
		}

		rule, queued, err := s.mappingService.CreateMerchantRule(c.Request.Context(), domain.MerchantRule{
			MCC:           req.MCC,
			Direction:     domain.Direction(req.Direction),
			Description:   req.Description,
			EzbCategoryID: req.EzbCategoryID,
		}, req.ApplyToMCC)
		if err != nil {
			logctx.From(c.Request.Context()).Warn("не вдалося створити правило продавця",
				"mcc", req.MCC, "description", req.Description, "err", err)
			s.respondError(c, err)
			return
		}

		logctx.From(c.Request.Context()).Info("створено правило продавця",
			"id", rule.ID, "mcc", rule.MCC, "description", rule.Description,
			"category_id", rule.EzbCategoryID, "apply_to_mcc", req.ApplyToMCC, "queued", queued)
		s.respondCreated(c, "", gin.H{"rule": merchantRuleToDTO(*rule), "queued": queued})
	}
}

// UpdateMerchantRuleRequest — тіло PUT /api/merchant-rules/:id.
type UpdateMerchantRuleRequest struct {
	EzbCategoryID string `json:"ezb_category_id" binding:"required"`
	Enabled       bool   `json:"enabled"`
}

// updateMerchantRule — PUT /api/merchant-rules/:id
func (s *Server) updateMerchantRule() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := s.pathID(c)
		if !ok {
			return
		}
		var req UpdateMerchantRuleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.respondError(c, err)
			return
		}

		rule, err := s.mappingService.UpdateMerchantRule(c.Request.Context(), id, req.EzbCategoryID, req.Enabled)
		if err != nil {
			logctx.From(c.Request.Context()).Warn("не вдалося оновити правило продавця", "id", id, "err", err)
			s.respondError(c, err)
			return
		}
		logctx.From(c.Request.Context()).Info("оновлено правило продавця",
			"id", rule.ID, "description", rule.Description, "category_id", rule.EzbCategoryID, "enabled", rule.Enabled)
		s.respondOK(c, gin.H{"rule": merchantRuleToDTO(*rule)})
	}
}

// ReviewItemDTO — транзакція на підтвердження (режим manual).
type ReviewItemDTO struct {
	ID          int64  `json:"id"`
	Source      string `json:"source"`
	AccountID   string `json:"account_id"`
	Description string `json:"description"`
	Amount      int64  `json:"amount"` // копійки, мінус — витрата
	TxTime      string `json:"tx_time"`
	MCC         int    `json:"mcc"`
	Direction   string `json:"direction"`
	// SuggestedCategoryID порожній — пропонувати нічого
	SuggestedCategoryID string `json:"suggested_category_id"`
	SuggestionSource    string `json:"suggestion_source"` // 'merchant_rule' | 'mcc_rule' | 'history' | ''
}

// reviewQueue — GET /api/review
func (s *Server) reviewQueue() gin.HandlerFunc {
	return func(c *gin.Context) {
		queue, err := s.mappingService.ReviewQueue(c.Request.Context())
		if err != nil {
			logctx.From(c.Request.Context()).Error("не вдалося отримати транзакції на підтвердження", "err", err)
			s.respondError(c, err)
			return
		}
		items := make([]ReviewItemDTO, 0, len(queue))
		for _, item := range queue {
			tx := item.Job.Tx
			items = append(items, ReviewItemDTO{
				ID:                  item.Job.ID,
				Source:              string(tx.Source),
				AccountID:           tx.AccountID,
				Description:         tx.Description,
				Amount:              tx.Amount,
				TxTime:              tx.Time.Format(time.RFC3339),
				MCC:                 tx.MCC,
				Direction:           string(tx.Direction()),
				SuggestedCategoryID: item.Suggestion.CategoryID,
				SuggestionSource:    string(item.Suggestion.Source),
			})
		}
		s.respondOK(c, gin.H{"items": items, "count": len(items)})
	}
}

// ConfirmJobRequest — тіло POST /api/jobs/:id/confirm.
type ConfirmJobRequest struct {
	EzbCategoryID string `json:"ezb_category_id" binding:"required"`
}

// confirmJob — POST /api/jobs/:id/confirm: людина погодила категорію, транзакція йде в ezBookkeeping.
func (s *Server) confirmJob() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := s.pathID(c)
		if !ok {
			return
		}
		var req ConfirmJobRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.respondError(c, err)
			return
		}

		if err := s.mappingService.ConfirmJob(c.Request.Context(), id, req.EzbCategoryID); err != nil {
			logctx.From(c.Request.Context()).Warn("не вдалося підтвердити транзакцію", "job", id, "err", err)
			s.respondError(c, err)
			return
		}
		logctx.From(c.Request.Context()).Info("транзакцію підтверджено", "job", id, "category_id", req.EzbCategoryID)
		s.respondOK(c, gin.H{"id": id})
	}
}

// pathID — додатний :id зі шляху; інакше відповідає 400 і повертає false.
func (s *Server) pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		s.errorResponse(c, http.StatusBadRequest, errBadID)
		return 0, false
	}
	return id, true
}
