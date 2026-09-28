package main

import (
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"livingspeech/internal/store"
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
	// Lets scripts (and the Homebrew test) check the installed app runs,
	// without starting the GUI.
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("LivingSpeech", appVersion())
		return
	}

	dataDir, err := store.DefaultDir()
	if err != nil {
		log.Fatal(err)
	}
	speech := NewSpeechService(dataDir)
	enableSpellChecking()

	app := application.New(application.Options{
		Name:        "LivingSpeech",
		Description: "Menu bar text-to-speech for OpenVox",
		Services: []application.Service{
			application.NewService(speech),
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
		Title:            "LivingSpeech",
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
	menu.Add("Quit LivingSpeech").OnClick(func(*application.Context) { app.Quit() })

	tray := app.SystemTray.New()
	tray.SetTemplateIcon(trayIcon)
	tray.SetTooltip("LivingSpeech")
	tray.SetMenu(menu)
	// Attaching lets Wails hide the panel when the right-click menu opens;
	// left clicks go to the panel so it can reopen where the user left it.
	tray.AttachWindow(window).WindowOffset(panelOffset)
	panel := NewPanel(app, window, tray, store.NewWindowStore(dataDir))

	speech.attach(app, window, panel)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
