package main

import (
	"context"
	"embed"
	"log/slog"
	"sync/atomic"

	"springlink/internal/tray"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed appicon.png
var appIcon []byte

func main() {
	tray.SetIcon(appIcon)

	app := NewApp()

	err := wails.Run(&options.App{
		Title:          "SpringLink",
		Width:          1024,
		Height:         768,
		Frameless:       true,
		CSSDragProperty: "--wails-draggable",
		CSSDragValue:    "drag",
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			if atomic.LoadInt32(&app.closing) == 1 {
				return false
			}
			atomic.StoreInt32(&app.closing, 1)
			runtime.EventsEmit(ctx, "window-close-requested", nil)
			return true
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		slog.Error("failed to start application", "error", err)
	}
}
