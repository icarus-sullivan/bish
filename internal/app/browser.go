package app

import "sync/atomic"

// Native browser views for the Preview tab (browser_impl_darwin.go). The
// frontend owns layout: it reports the stage rect and visibility, and the
// view emits "browser:nav" (address/title/loading/history) and
// "browser:key" (⌘-combos the page didn't keep) back.

// browserApp routes cgo callbacks, which carry no App, to the events bus.
var browserApp atomic.Pointer[App]

type BrowserNav struct {
	ID         string `json:"id"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Loading    bool   `json:"loading"`
	CanBack    bool   `json:"canBack"`
	CanForward bool   `json:"canForward"`
}

// BrowserSupported reports whether this platform has the native view; the
// Preview tab uses its iframe when it doesn't.
func (a *App) BrowserSupported() bool { return browserSupported }

func (a *App) BrowserOpen(id, url string) {
	browserApp.Store(a)
	browserOpen(id, url)
}

func (a *App) BrowserSetFrame(id string, x, y, w, h float64) { browserSetFrame(id, x, y, w, h) }
func (a *App) BrowserSetVisible(id string, visible bool)     { browserSetVisible(id, visible) }
func (a *App) BrowserNavigate(id, url string)                { browserNavigate(id, url) }
func (a *App) BrowserReload(id string)                       { browserCmd(id, 0) }
func (a *App) BrowserBack(id string)                         { browserCmd(id, 1) }
func (a *App) BrowserForward(id string)                      { browserCmd(id, 2) }
func (a *App) BrowserClose(id string)                        { browserClose(id) }
