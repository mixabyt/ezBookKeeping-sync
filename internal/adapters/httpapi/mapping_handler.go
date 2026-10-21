package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"ezbkTools/internal/domain"
	"ezbkTools/internal/platform/logctx"
	"github.com/gin-gonic/gin"
)

// UnmappedKeyDTO — ключ без мапінгу у вигляді, зручному для UI.
type UnmappedKeyDTO struct {
	Source     string `json:"source"`      // 'mono' | 'privat'
	KeyType    string `json:"key_type"`    // 'mcc' | 'bank_category'
	Key        string `json:"key"`         // '5411' або назва категорії банку
	Direction  string `json:"direction"`   // 'expense' | 'income'
	SampleDesc string `json:"sample_desc"` // опис однієї з транзакцій — щоб зрозуміти, що це
	Hits       int    `json:"hits"`
	FirstSeen  string `json:"first_seen"`
	LastSeen   string `json:"last_seen"`
	Resolved   bool   `json:"resolved"` // правило вже є, показуємо як історію
}

func unmappedToDTO(key domain.UnmappedKey) UnmappedKeyDTO {
	return UnmappedKeyDTO{
		Source:     string(key.Source),
		KeyType:    string(key.KeyType),
		Key:        key.Key,
		Direction:  string(key.Direction),
		SampleDesc: key.SampleDesc,
		Hits:       key.Hits,
		FirstSeen:  key.FirstSeen.Format(time.RFC3339),
		LastSeen:   key.LastSeen.Format(time.RFC3339),
		Resolved:   key.Resolved,
	}
}

// unmappedKeys — GET /api/unmapped?key_type=mcc&only_pending=true&limit=100
// Віддає ключі, які траплялись у транзакціях без мапінгу, найчастіші першими.
func (s *Server) unmappedKeys() gin.HandlerFunc {
	return func(c *gin.Context) {
		filter := domain.UnmappedFilter{
			KeyType:     domain.KeyType(c.Query("key_type")),
			OnlyPending: c.Query("only_pending") == "true",
		}

		switch filter.KeyType {
		case "", domain.KeyMCC, domain.KeyBankCategory:
		default:
			s.errorResponse(c, http.StatusBadRequest, errBadKeyType)
			return
		}

		if raw := c.Query("limit"); raw != "" {
			limit, err := strconv.Atoi(raw)
			if err != nil || limit <= 0 {
				s.errorResponse(c, http.StatusBadRequest, errBadLimit)
				return
			}
			filter.Limit = limit
		}

		keys, err := s.mappingService.UnmappedKeys(c.Request.Context(), filter)
		if err != nil {
			logctx.From(c.Request.Context()).Error("не вдалося отримати unmapped keys", "err", err)
			s.respondError(c, err)
			return
		}

		items := make([]UnmappedKeyDTO, 0, len(keys))
		for _, key := range keys {
			items = append(items, unmappedToDTO(key))
		}
		s.respondOK(c, gin.H{"items": items, "count": len(items)})
	}
}

// CreateRuleRequest — тіло POST /api/rules.
// MCCTo не обовʼязковий: для одного MCC достатньо надіслати лише mcc_from.
type CreateRuleRequest struct {
	MCCFrom       int    `json:"mcc_from" binding:"required"`
	MCCTo         *int   `json:"mcc_to"`
	Direction     string `json:"direction" binding:"required"`
	EzbCategoryID string `json:"ezb_category_id" binding:"required"`
	Priority      int    `json:"priority"`
	Note          string `json:"note"`
}

// RuleDTO — створене правило у відповіді.
type RuleDTO struct {
	ID            int64  `json:"id"`
	MCCFrom       int    `json:"mcc_from"`
	MCCTo         int    `json:"mcc_to"`
	Direction     string `json:"direction"`
	Priority      int    `json:"priority"`
	EzbCategoryID string `json:"ezb_category_id"`
	Enabled       bool   `json:"enabled"`
	Note          string `json:"note"`
}

// createRule — POST /api/rules: мапить MCC (або діапазон) на категорію ezBookkeeping.
// Після цього відповідний ключ у /api/unmapped стає resolved.
func (s *Server) createRule() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateRuleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.respondError(c, err)
			return
		}

		mccTo := req.MCCFrom
		if req.MCCTo != nil {
			mccTo = *req.MCCTo
		}

		rule, err := s.mappingService.CreateRule(c.Request.Context(), domain.MCCCategoryRule{
			MCCFrom:       req.MCCFrom,
			MCCTo:         mccTo,
			Direction:     domain.Direction(req.Direction),
			Priority:      req.Priority,
			EzbCategoryID: req.EzbCategoryID,
			Enabled:       true,
			Note:          req.Note,
		})
		if err != nil {
			logctx.From(c.Request.Context()).Warn("не вдалося створити правило",
				"mcc_from", req.MCCFrom, "direction", req.Direction, "err", err)
			s.respondError(c, err)
			return
		}

		logctx.From(c.Request.Context()).Info("створено правило",
			"id", rule.ID, "mcc", rule.MCCFrom, "mcc_to", rule.MCCTo,
			"direction", rule.Direction, "category_id", rule.EzbCategoryID)

		s.respondCreated(c, "", RuleDTO{
			ID:            rule.ID,
			MCCFrom:       rule.MCCFrom,
			MCCTo:         rule.MCCTo,
			Direction:     string(rule.Direction),
			Priority:      rule.Priority,
			EzbCategoryID: rule.EzbCategoryID,
			Enabled:       rule.Enabled,
			Note:          rule.Note,
		})
	}
}

// EzCategoryDTO — категорія ezBookkeeping для випадайки в UI.
type EzCategoryDTO struct {
	ID         string `json:"id"` // це значення йде в ezb_category_id правила
	Name       string `json:"name"`
	ParentID   string `json:"parent_id"`
	ParentName string `json:"parent_name"`
	Kind       string `json:"kind"` // 'income' | 'expense' | 'transfer'
	Hidden     bool   `json:"hidden"`
}

// ezCategories — GET /api/categories?kind=expense&include_hidden=true
// Проксі до ezBookkeeping: ключ лишається на сервері, у браузер не потрапляє.
func (s *Server) ezCategories() gin.HandlerFunc {
	return func(c *gin.Context) {
		filter := domain.EzCategoryFilter{
			Kind:          domain.EzCategoryKind(c.Query("kind")),
			IncludeHidden: c.Query("include_hidden") == "true",
		}

		switch filter.Kind {
		case "", domain.EzCategoryIncome, domain.EzCategoryExpense, domain.EzCategoryTransfer:
		default:
			s.errorResponse(c, http.StatusBadRequest, errBadKind)
			return
		}

		categories, err := s.mappingService.EzCategories(c.Request.Context(), filter)
		if err != nil {
			logctx.From(c.Request.Context()).Error("не вдалося отримати категорії ezBookkeeping", "err", err)
			s.respondError(c, err)
			return
		}

		items := make([]EzCategoryDTO, 0, len(categories))
		for _, category := range categories {
			items = append(items, EzCategoryDTO{
				ID:         category.ID,
				Name:       category.Name,
				ParentID:   category.ParentID,
				ParentName: category.ParentName,
				Kind:       string(category.Kind),
				Hidden:     category.Hidden,
			})
		}
		s.respondOK(c, gin.H{"items": items, "count": len(items)})
	}
}
