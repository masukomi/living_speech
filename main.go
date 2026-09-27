package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"voxbox/internal/store"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/trayicon.png
var trayIcon []byte

const (
	panelWidth  = 380
	panelOffset = 4
)

func main() {
	dataDir, err := store.DefaultDir()
	if err != nil {
		log.Fatal(err)
	}
	vox := NewVoxService(dataDir)

	app := application.New(application.Options{
		Name:        "VoxBox",
		Description: "Menu bar text-to-speech for OpenVox",
		Services: []application.Service{
			application.NewService(vox),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// Menu bar only: no Dock icon.
			ActivationPolicy: application.ActivationPolicyAccessory,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "panel",
		Title:            "VoxBox",
		Width:            panelWidth,
		Height:           240,
		Frameless:        true,
		AlwaysOnTop:      true,
		DisableResize:    true,
		Hidden:           true,
		HideOnEscape:     true,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		Mac: application.MacWindow{
			Backdrop:     application.MacBackdropTranslucent,
			TitleBar:     application.MacTitleBarHiddenInset,
			CornerRadius: 14,
		},
		URL: "/",
	})

	menu := app.NewMenu()
	menu.Add("Quit VoxBox").OnClick(func(*application.Context) { app.Quit() })

	tray := app.SystemTray.New()
	tray.SetTemplateIcon(trayIcon)
	tray.SetTooltip("VoxBox")
	tray.SetMenu(menu)
	tray.AttachWindow(window).WindowOffset(panelOffset)

	vox.attach(app, window, tray)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
