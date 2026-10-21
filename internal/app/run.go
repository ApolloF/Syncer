package app

import (
	"context"
	"io/fs"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/ApolloF/syncer/internal/logx"
	"github.com/ApolloF/syncer/internal/meta"
)

// version is this build's version, handed in by main (set at release builds).
var version = "dev"

// Options is what main hands to Run: the embedded files and the values set
// at link time.
type Options struct {
	Assets             fs.FS  // the built frontend
	TrayIcon           []byte // the notification-area icon
	Version            string // "" or "dev" for a build that isn't a release
	GdriveClientID     string
	GdriveClientSecret string
}

// apply takes over the values main handed in.
func apply(o Options) {
	if o.Version != "" {
		version = o.Version
	}
	meta.AppVersion = version
	gdriveClientID, gdriveClientSecret = o.GdriveClientID, o.GdriveClientSecret
	trayIcon = o.TrayIcon
}

// Run starts Syncer as the command line asks: a window, or one of the
// windowless runs.
func Run(o Options) {
	apply(o)
	logx.RecordCrashes()
	startHidden := false
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, afterUpdateArg) { // started by the Syncer it updated
			waitForUpdated(a)
			continue
		}
		switch a {
		case "--tray": // started with Windows: only the tray icon, no window
			startHidden = true
		case "--background", "--backup":
			runBackground()
			return
		case "--api": // the launcher API without a window (see api.go)
			runAPI()
			return
		case "--resume": // a pause ran out (\Syncer-Resume task)
			runResume()
			return
		case "--uninstall": // called by the uninstaller
			runUninstall()
			return
		}
	}

	app := NewApp()
	app.hidden = startHidden
	err := wails.Run(&options.App{
		Title:     "Syncer",
		Width:     1040,
		Height:    700,
		MinWidth:  760,
		MinHeight: 520,
		AssetServer: &assetserver.Options{
			Assets: o.Assets,
		},
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		StartHidden:      startHidden,
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			go logx.Run("launcher API", func() { serveAPI(ctx, app) })
		},
		OnBeforeClose: app.beforeClose,
		OnShutdown:    app.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "b1c9c8a4-syncer-apollof",
			// Opening Syncer again just brings the existing window forward.
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				app.showWindow()
			},
		},
		Bind: []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			BackdropType:         windows.Mica,
			Theme:                windows.SystemDefault,
		},
	})
	if err != nil {
		logx.Printf("fatal: %v", err)
	}
}
