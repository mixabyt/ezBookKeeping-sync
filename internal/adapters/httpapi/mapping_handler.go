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
	// WaitingJobs — скільки транзакцій зараз чекають саме на цей ключ
	WaitingJobs int    `json:"waiting_jobs"`
	FirstSeen   string `json:"first_seen"`
	LastSeen    string `json:"last_seen"`
	Resolved    bool   `json:"resolved"` // правило вже є, показуємо як історію
}

func unmappedToDTO(key domain.UnmappedKey) UnmappedKeyDTO {
	return UnmappedKeyDTO{
		Source:      string(key.Source),
		KeyType:     string(key.KeyType),
		Key:         key.Key,
		Direction:   string(key.Direction),
		SampleDesc:  key.SampleDesc,
		Hits:        key.Hits,
		WaitingJobs: key.WaitingJobs,
		FirstSeen:   key.FirstSeen.Format(time.RFC3339),
		LastSeen:    key.LastSeen.Format(time.RFC3339),
		Resolved:    key.Resolved,
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

		rule, queued, err := s.mappingService.CreateRule(c.Request.Context(), domain.MCCCategoryRule{
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
			"direction", rule.Direction, "category_id", rule.EzbCategoryID, "queued", queued)

		s.respondCreated(c, "", gin.H{"rule": ruleToDTO(*rule), "queued": queued})
	}
}

func ruleToDTO(rule domain.MCCCategoryRule) RuleDTO {
	return RuleDTO{
		ID:            rule.ID,
		MCCFrom:       rule.MCCFrom,
		MCCTo:         rule.MCCTo,
		Direction:     string(rule.Direction),
		Priority:      rule.Priority,
		EzbCategoryID: rule.EzbCategoryID,
		Enabled:       rule.Enabled,
		Note:          rule.Note,
	}
}

// listRules — GET /api/rules: усі правила, включно з вимкненими.
func (s *Server) listRules() gin.HandlerFunc {
	return func(c *gin.Context) {
		rules, err := s.mappingService.Rules(c.Request.Context())
		if err != nil {
			logctx.From(c.Request.Context()).Error("не вдалося отримати правила", "err", err)
			s.respondError(c, err)
			return
		}

		items := make([]RuleDTO, 0, len(rules))
		for _, rule := range rules {
			items = append(items, ruleToDTO(rule))
		}
		s.respondOK(c, gin.H{"items": items, "count": len(items)})
	}
}

// UpdateRuleRequest — тіло PUT /api/rules/:id. Діапазон і напрямок не змінюються.
type UpdateRuleRequest struct {
	EzbCategoryID string `json:"ezb_category_id" binding:"required"`
	Enabled       bool   `json:"enabled"`
	Note          string `json:"note"`
}

// updateRule — PUT /api/rules/:id: інша категорія, увімкнути/вимкнути, нотатка.
func (s *Server) updateRule() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			s.errorResponse(c, http.StatusBadRequest, errBadID)
			return
		}

		var req UpdateRuleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.respondError(c, err)
			return
		}

		rule, queued, err := s.mappingService.UpdateRule(c.Request.Context(), id, req.EzbCategoryID, req.Enabled, req.Note)
		if err != nil {
			logctx.From(c.Request.Context()).Warn("не вдалося оновити правило", "id", id, "err", err)
			s.respondError(c, err)
			return
		}

		logctx.From(c.Request.Context()).Info("оновлено правило",
			"id", rule.ID, "mcc", rule.MCCFrom, "mcc_to", rule.MCCTo, "direction", rule.Direction,
			"category_id", rule.EzbCategoryID, "enabled", rule.Enabled, "queued", queued)

		s.respondOK(c, gin.H{"rule": ruleToDTO(*rule), "queued": queued})
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

// EzAccountDTO — рахунок ezBookkeeping для випадайки в UI.
type EzAccountDTO struct {
	ID         string `json:"id"` // це значення йде в ezb_account_id мапінгу
	Name       string `json:"name"`
	ParentName string `json:"parent_name"`
	Currency   string `json:"currency"`
}

// ezAccounts — GET /api/accounts: довідник рахунків ezBookkeeping (ключ лишається на сервері).
func (s *Server) ezAccounts() gin.HandlerFunc {
	return func(c *gin.Context) {
		accounts, err := s.mappingService.EzAccounts(c.Request.Context())
		if err != nil {
			logctx.From(c.Request.Context()).Error("не вдалося отримати рахунки ezBookkeeping", "err", err)
			s.respondError(c, err)
			return
		}

		items := make([]EzAccountDTO, 0, len(accounts))
		for _, a := range accounts {
			items = append(items, EzAccountDTO{ID: a.ID, Name: a.Name, ParentName: a.ParentName, Currency: a.Currency})
		}
		s.respondOK(c, gin.H{"items": items, "count": len(items)})
	}
}

// WaitingAccountDTO — банківський рахунок без мапінгу, що блокує транзакції.
type WaitingAccountDTO struct {
	Source        string `json:"source"`
	BankAccountID string `json:"bank_account_id"`
	WaitingJobs   int    `json:"waiting_jobs"`
	SampleDesc    string `json:"sample_desc"`
}

// unmappedAccounts — GET /api/unmapped-accounts
func (s *Server) unmappedAccounts() gin.HandlerFunc {
	return func(c *gin.Context) {
		waiting, err := s.mappingService.WaitingAccounts(c.Request.Context())
		if err != nil {
			logctx.From(c.Request.Context()).Error("не вдалося отримати незамаплені рахунки", "err", err)
			s.respondError(c, err)
			return
		}

		items := make([]WaitingAccountDTO, 0, len(waiting))
		for _, w := range waiting {
			items = append(items, WaitingAccountDTO{
				Source:        string(w.Source),
				BankAccountID: w.BankAccountID,
				WaitingJobs:   w.WaitingJobs,
				SampleDesc:    w.SampleDesc,
			})
		}
		s.respondOK(c, gin.H{"items": items, "count": len(items)})
	}
}

// AccountMappingDTO — збережений мапінг банківського рахунку.
type AccountMappingDTO struct {
	Source        string `json:"source"`
	BankAccountID string `json:"bank_account_id"`
	EzbAccountID  string `json:"ezb_account_id"`
	Name          string `json:"name"`
}

// listAccountMappings — GET /api/account-mappings. Редагування — тим самим POST (upsert).
func (s *Server) listAccountMappings() gin.HandlerFunc {
	return func(c *gin.Context) {
		mappings, err := s.mappingService.AccountMappings(c.Request.Context())
		if err != nil {
			logctx.From(c.Request.Context()).Error("не вдалося отримати мапінги рахунків", "err", err)
			s.respondError(c, err)
			return
		}

		items := make([]AccountMappingDTO, 0, len(mappings))
		for _, m := range mappings {
			items = append(items, AccountMappingDTO{
				Source:        string(m.Source),
				BankAccountID: m.BankAccountID,
				EzbAccountID:  m.EzbAccountID,
				Name:          m.Name,
			})
		}
		s.respondOK(c, gin.H{"items": items, "count": len(items)})
	}
}

// CreateAccountMappingRequest — тіло POST /api/account-mappings.
type CreateAccountMappingRequest struct {
	Source        string `json:"source" binding:"required"`
	BankAccountID string `json:"bank_account_id" binding:"required"`
	EzbAccountID  string `json:"ezb_account_id" binding:"required"`
	Name          string `json:"name"`
}

// createAccountMapping — POST /api/account-mappings: банківський рахунок → рахунок ezBookkeeping.
// Повторний запит для того самого рахунку перезаписує мапінг — так виправляється помилковий вибір.
func (s *Server) createAccountMapping() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateAccountMappingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			s.respondError(c, err)
			return
		}

		mapping := domain.AccountMapping{
			Source:        domain.Source(req.Source),
			BankAccountID: req.BankAccountID,
			EzbAccountID:  req.EzbAccountID,
			Name:          req.Name,
		}
		queued, err := s.mappingService.CreateAccountMapping(c.Request.Context(), mapping)
		if err != nil {
			logctx.From(c.Request.Context()).Warn("не вдалося зберегти мапінг рахунку",
				"source", req.Source, "bank_account_id", req.BankAccountID, "err", err)
			s.respondError(c, err)
			return
		}

		logctx.From(c.Request.Context()).Info("збережено мапінг рахунку",
			"source", req.Source, "bank_account_id", req.BankAccountID,
			"ezb_account_id", req.EzbAccountID, "queued", queued)

		s.respondCreated(c, "", gin.H{
			"mapping": gin.H{
				"source":          req.Source,
				"bank_account_id": req.BankAccountID,
				"ezb_account_id":  req.EzbAccountID,
				"name":            req.Name,
			},
			"queued": queued,
		})
	}
}
