package httpapi

import (
	"io/fs"
	"mime"
	"net/http"
	"strings"

	"ezbkTools/web"
	"github.com/gin-gonic/gin"
)

// serveWeb віддає PWA з вшитої файлової системи.
//
// NoRoute, а не Static("/"), бо інакше gin конфліктує з уже зареєстрованими
// /api та /mono-webhook. Невідомі GET-шляхи віддають index.html — щоб
// переходи всередині застосунку й перезавантаження сторінки не давали 404.
func (s *Server) serveWeb() gin.HandlerFunc {
	assets := web.FS()
	fileServer := http.FileServer(http.FS(assets))
	// Go не знає .webmanifest і віддав би text/plain
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")

	return func(c *gin.Context) {
		path := strings.TrimPrefix(c.Request.URL.Path, "/")

		if c.Request.Method != http.MethodGet || strings.HasPrefix(path, "api/") {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		if path != "" {
			if _, err := fs.Stat(assets, path); err == nil {
				// service worker має оновлюватись одразу, інакше стара оболонка
				// житиме доти, доки браузер сам не вирішить її перевірити
				if path == "sw.js" {
					c.Header("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}

		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}
