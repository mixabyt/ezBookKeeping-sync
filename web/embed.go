// Package web — статика PWA, вшита в бінарник.
// Один файл на деплой: ніякого npm і окремої роздачі через nginx.
package web

import (
	"embed"
	"io/fs"
)

//go:embed public
var files embed.FS

// FS — коренева файлова система застосунку (вміст public/).
func FS() fs.FS {
	sub, err := fs.Sub(files, "public")
	if err != nil {
		panic("web: не вдалося відкрити public: " + err.Error())
	}
	return sub
}
