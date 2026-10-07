package app

/*
#include <stdlib.h>
// Forward declarations only — definitions live in browser_impl_darwin.go
void bishBrowserOpenC(char* bid, char* url);
void bishBrowserSetFrameC(char* bid, double x, double y, double w, double h);
void bishBrowserSetVisibleC(char* bid, int visible);
void bishBrowserNavigateC(char* bid, char* url);
void bishBrowserCmdC(char* bid, int op);
void bishBrowserCloseC(char* bid);
*/
import "C"

import (
	"encoding/json"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const browserSupported = true

//export bishBrowserNavGo
func bishBrowserNavGo(bid, url, title *C.char, loading, canBack, canFwd C.int) {
	a := browserApp.Load()
	if a == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "browser:nav", BrowserNav{
		ID:         C.GoString(bid),
		URL:        C.GoString(url),
		Title:      C.GoString(title),
		Loading:    loading != 0,
		CanBack:    canBack != 0,
		CanForward: canFwd != 0,
	})
}

//export bishBrowserKeyGo
func bishBrowserKeyGo(bid, js *C.char) {
	a := browserApp.Load()
	if a == nil {
		return
	}
	var key map[string]any
	if json.Unmarshal([]byte(C.GoString(js)), &key) != nil {
		return
	}
	runtime.EventsEmit(a.ctx, "browser:key", key)
}

func withCStr(s string, f func(*C.char)) {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	f(cs)
}

func browserOpen(id, url string) {
	withCStr(id, func(cid *C.char) {
		withCStr(url, func(curl *C.char) { C.bishBrowserOpenC(cid, curl) })
	})
}

func browserSetFrame(id string, x, y, w, h float64) {
	withCStr(id, func(cid *C.char) {
		C.bishBrowserSetFrameC(cid, C.double(x), C.double(y), C.double(w), C.double(h))
	})
}

func browserSetVisible(id string, visible bool) {
	v := C.int(0)
	if visible {
		v = 1
	}
	withCStr(id, func(cid *C.char) { C.bishBrowserSetVisibleC(cid, v) })
}

func browserNavigate(id, url string) {
	withCStr(id, func(cid *C.char) {
		withCStr(url, func(curl *C.char) { C.bishBrowserNavigateC(cid, curl) })
	})
}

func browserCmd(id string, op int) {
	withCStr(id, func(cid *C.char) { C.bishBrowserCmdC(cid, C.int(op)) })
}

func browserClose(id string) {
	withCStr(id, func(cid *C.char) { C.bishBrowserCloseC(cid) })
}
