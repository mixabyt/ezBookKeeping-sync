package httpapi

import (
	"ezbkTools/internal/app"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"net/http"
)

type Server struct {
	router      *gin.Engine
	syncService *app.SyncService
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	s.router.ServeHTTP(writer, request)
}

func NewServer(syncService *app.SyncService) *Server {
	s := &Server{
		router:      gin.Default(),
		syncService: syncService,
	}
	s.init()
	return s
}

func (s *Server) init() {
	s.router.Use(requestid.New())

	s.router.GET("/mono-webhook", s.registerWebHook())
	s.router.POST("/mono-webhook", s.monoWebHook())
}
