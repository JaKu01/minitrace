package server

import (
	"embed"
	"io/fs"
)

// embeddedWeb contains the Angular production build. The Docker build replaces
// the fallback page in server/web before compiling the server binary.
//
//go:embed web
var embeddedWeb embed.FS

func EmbeddedApp() fs.FS {
	app, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		panic(err)
	}
	return app
}
