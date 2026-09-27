package main

import (
	"log"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"livingspeech/internal/store"
)

// Panel shows and hides the speech window from the menu bar, remembering
// where the user last put it.
type Panel struct {
	app    *application.App
	window *application.WebviewWindow
	tray   *application.SystemTray
	store  *store.WindowStore

	mu        sync.Mutex
	shown     bool // shown at least once this run, so its position is meaningful
	saveTimer *time.Timer
}

func NewPanel(app *application.App, window *application.WebviewWindow, tray *application.SystemTray, ws *store.WindowStore) *Panel {
	p := &Panel{app: app, window: window, tray: tray, store: ws}
	tray.OnClick(p.Toggle)
	window.OnWindowEvent(events.Common.WindowDidMove, func(*application.WindowEvent) { p.scheduleSave() })
	window.OnWindowEvent(events.Common.WindowHide, func(*application.WindowEvent) { p.saveNow() })
	app.OnShutdown(p.saveNow)
	return p
}

// Toggle shows the panel at its remembered position (or under the menu bar
// icon the first time), or hides it if it's already showing.
func (p *Panel) Toggle() {
	if p.window.IsVisible() {
		p.window.Hide()
		return
	}
	if pos := p.savedPosition(); pos != nil {
		p.window.SetPosition(pos.X, pos.Y)
	} else if err := p.tray.PositionWindow(p.window, panelOffset); err != nil {
		log.Printf("positioning panel: %v", err)
	}
	p.mu.Lock()
	p.shown = true
	p.mu.Unlock()
	p.window.Show().Focus()
}

func (p *Panel) Hide() {
	p.window.Hide()
}

// SetSize resizes the panel, keeping its top-left corner in place.
func (p *Panel) SetSize(width, height int) {
	if w, h := p.window.Size(); w == width && h == height {
		return
	}
	setSizeKeepingTopLeft(p.window, width, height)
}

// savedPosition returns the remembered position if it's still on a connected
// screen, so the panel can't get lost after a display is unplugged.
func (p *Panel) savedPosition() *store.WindowPosition {
	pos, err := p.store.Load()
	if err != nil {
		log.Printf("loading window position: %v", err)
		return nil
	}
	if pos == nil {
		return nil
	}
	// Require the top strip of the panel (where you grab it) to be on screen.
	x, y := pos.X+40, pos.Y+10
	for _, scr := range p.app.Screen.GetAll() {
		b := scr.Bounds
		if x >= b.X && x < b.X+b.Width && y >= b.Y && y < b.Y+b.Height {
			return pos
		}
	}
	return nil
}

// Moves arrive continuously while dragging, so save once things settle.
func (p *Panel) scheduleSave() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.shown {
		return
	}
	if p.saveTimer != nil {
		p.saveTimer.Stop()
	}
	p.saveTimer = time.AfterFunc(400*time.Millisecond, p.saveNow)
}

func (p *Panel) saveNow() {
	p.mu.Lock()
	if p.saveTimer != nil {
		p.saveTimer.Stop()
		p.saveTimer = nil
	}
	shown := p.shown
	p.mu.Unlock()
	if !shown {
		return
	}
	x, y := p.window.Position()
	if err := p.store.Save(store.WindowPosition{X: x, Y: y}); err != nil {
		log.Printf("saving window position: %v", err)
	}
}
