package main

/*
#include <stdlib.h>
// Forward declarations only — definitions live in dock_impl_darwin.go
void setBishDockMenuC(char** paths, char** names, int n);
*/
import "C"

import (
	"unsafe"

	"github.com/csullivan/bish/internal/project"
)

//export goOpenRecentDock
func goOpenRecentDock(path *C.char) {
	if globalApp != nil {
		go globalApp.OpenRecentInNewWindow(C.GoString(path)) //nolint
	}
}

func setBishDockMenuFromRecents(entries []*project.RecentEntry) {
	n := len(entries)
	if n > 8 {
		n = 8
	}
	if n == 0 {
		return
	}
	cPaths := make([]*C.char, n)
	cNames := make([]*C.char, n)
	for i, e := range entries[:n] {
		cPaths[i] = C.CString(e.Path)
		cNames[i] = C.CString(e.Name)
	}
	// setBishDockMenuC converts to NSStrings before dispatch_async, so freeing here is safe.
	C.setBishDockMenuC((**C.char)(unsafe.Pointer(&cPaths[0])), (**C.char)(unsafe.Pointer(&cNames[0])), C.int(n))
	for i := 0; i < n; i++ {
		C.free(unsafe.Pointer(cPaths[i]))
		C.free(unsafe.Pointer(cNames[i]))
	}
}
