package httpapi

import (
	"ezbkTools/internal/app"
	"net/http"
	"time"

	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
)

// скільки невдалих спроб входу дозволено з одного IP і за який період
const (
	maxLoginAttempts = 10
	loginWindow      = 15 * time.Minute
)

type Server struct {
	router         *gin.Engine
	syncService    *app.SyncService
	mappingService *app.MappingService
	auth           AuthConfig
	attempts       *cache.Cache
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	s.router.ServeHTTP(writer, request)
}

func NewServer(syncService *app.SyncService, mappingService *app.MappingService, auth AuthConfig) *Server {
	s := &Server{
		router:         gin.Default(),
		syncService:    syncService,
		mappingService: mappingService,
		auth:           auth,
		attempts:       cache.New(loginWindow, loginWindow),
	}
	s.init()
	return s
}

// loginAllowed / failedLogin — найпростіший захист від перебору пароля:
// лічильник невдалих спроб на IP, який сам протухає через loginWindow.
func (s *Server) loginAllowed(ip string) bool {
	v, found := s.attempts.Get(ip)
	return !found || v.(int) < maxLoginAttempts
}

func (s *Server) failedLogin(ip string) {
	if _, found := s.attempts.Get(ip); !found {
		s.attempts.Set(ip, 1, cache.DefaultExpiration)
		return
	}
	_, _ = s.attempts.IncrementInt(ip, 1)
}

func (s *Server) init() {
	s.router.Use(requestid.New())

	s.router.GET("/mono-webhook", s.registerWebHook())
	s.router.POST("/mono-webhook", s.checkMonoSign(), s.monoWebHook())

	// API для PWA: вхід відкритий, решта — лише з валідним токеном
	api := s.router.Group("/api")
	api.POST("/login", s.login())
	api.POST("/logout", s.logout())

	secured := api.Group("", s.requireAuth())
	secured.GET("/me", func(c *gin.Context) {
		s.respondOK(c, gin.H{"login": c.GetString("login")})
	})
	secured.GET("/unmapped", s.unmappedKeys())
	secured.GET("/rules", s.listRules())
	secured.POST("/rules", s.createRule())
	secured.PUT("/rules/:id", s.updateRule())
	secured.GET("/account-mappings", s.listAccountMappings())

	// режими мапінгу: налаштування, правила продавців, ручне підтвердження
	secured.GET("/settings", s.getSettings())
	secured.PUT("/settings", s.putSettings())
	secured.GET("/waiting-merchants", s.waitingMerchants())
	secured.GET("/merchant-rules", s.listMerchantRules())
	secured.POST("/merchant-rules", s.createMerchantRule())
	secured.PUT("/merchant-rules/:id", s.updateMerchantRule())
	secured.GET("/review", s.reviewQueue())
	secured.POST("/jobs/:id/confirm", s.confirmJob())
	secured.GET("/categories", s.ezCategories())
	secured.GET("/accounts", s.ezAccounts())
	secured.GET("/unmapped-accounts", s.unmappedAccounts())
	secured.POST("/account-mappings", s.createAccountMapping())

	// усе інше — статика PWA
	s.router.NoRoute(s.serveWeb())
}
