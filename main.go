package main

import (
	"embed"

	"github.com/ApolloF/syncer/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/tray.ico
var trayIcon []byte

// Set at release builds (-ldflags "-X main.version=v1.2.3"); the Google
// client only in personal builds (see internal/app/gdrive.go).
var (
	version            = "dev"
	gdriveClientID     = ""
	gdriveClientSecret = ""
)

func main() {
	app.Run(app.Options{
		Assets:             assets,
		TrayIcon:           trayIcon,
		Version:            version,
		GdriveClientID:     gdriveClientID,
		GdriveClientSecret: gdriveClientSecret,
	})
}
