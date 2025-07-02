package main

import (
	"embed"
	"os"

	"Gleip/backend"
	"Gleip/backend/network"
	"Gleip/backend/paths"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Initialize settings first
	if err := backend.InitSettings(); err != nil {
		println("Error initializing settings:", err.Error())
	}

	// Initialize paths
	if err := paths.InitPaths(); err != nil {
		println("Error initializing paths:", err.Error())
	}

	// Create an instance of the app structure
	app := backend.NewApp()

	// Add these lines before app.Run()
	backend.StartDebugServer()
	backend.StartMemoryMonitor()

	// Initialize telemetry after settings have been loaded
	backend.InitTelemetry()
	// Ensure telemetry is properly shutdown when the app closes
	defer backend.ShutdownTelemetry()

	// Initialize the settings controller
	settingsController := backend.NewSettingsController()

	// Initialize HTTP helper for frontend
	httpHelper := network.NewHTTPHelper()

	// Configure debug options - only enable in development mode
	debugOptions := options.Debug{}
	if os.Getenv("GLEIP_DEV_MODE") == "true" {
		debugOptions.OpenInspectorOnStartup = true
	}

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "Gleip",
		Width:     1200,
		Height:    800,
		MinWidth:  800,
		MinHeight: 600,
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     false,
			DisableWebViewDrop: true,
			CSSDropProperty:    "--wails-drop-target",
			CSSDropValue:       "drop",
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.Startup,
		OnShutdown:       app.Shutdown,

		Bind: []interface{}{
			app,
			settingsController,
			httpHelper,
		},
		LogLevel:           logger.DEBUG,
		LogLevelProduction: logger.ERROR,
		Mac:                &mac.Options{},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
		Linux: &linux.Options{},
		Debug: debugOptions,
	})

	if err != nil {
		// Track app crash if possible
		backend.TrackAppCrash(err)
		println("Error:", err.Error())
	}
}
