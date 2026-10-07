//go:build !darwin

package app

// No native preview view off macOS yet: the Preview tab falls back to its
// iframe (BrowserSupported() == false), so these are never reached.
const browserSupported = false

func browserOpen(id, url string)                    {}
func browserSetFrame(id string, x, y, w, h float64) {}
func browserSetVisible(id string, visible bool)     {}
func browserNavigate(id, url string)                {}
func browserCmd(id string, op int)                  {}
func browserClose(id string)                        {}
