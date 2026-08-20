package main

/*
#include <stdlib.h>
// Forward declaration only — definition lives in dock_impl_darwin.go
void setBishDockMenuC(char** projPaths, char** projNames, int projN,
                       char** filePaths, char** fileNames, int fileN);
*/
import "C"

import (
	"unsafe"

	"github.com/csullivan/bish/internal/project"
)

//export goOpenRecentDock
func goOpenRecentDock(path *C.char) {
	if globalApp != nil {
		// OpenRecentProject focuses the project's window if it's already
		// open, otherwise spawns one — same entry point File > Open Recent
		// uses (buildMenu in main.go), so the Dock menu can't diverge again.
		go globalApp.OpenRecentProject(C.GoString(path)) //nolint
	}
}

//export goOpenRecentFileDock
func goOpenRecentFileDock(path *C.char) {
	if globalApp != nil {
		go globalApp.OpenRecentFileInNewWindow(C.GoString(path)) //nolint
	}
}

func setBishDockMenuFromRecents(projects []*project.RecentEntry, files []*project.RecentFile) {
	if len(projects) == 0 && len(files) == 0 {
		return
	}
	pn := len(projects)
	if pn > 8 {
		pn = 8
	}
	fn := len(files)
	if fn > 8 {
		fn = 8
	}

	projPaths := make([]*C.char, pn)
	projNames := make([]*C.char, pn)
	for i := 0; i < pn; i++ {
		projPaths[i] = C.CString(projects[i].Path)
		projNames[i] = C.CString(projects[i].Name)
	}
	filePaths := make([]*C.char, fn)
	fileNames := make([]*C.char, fn)
	for i := 0; i < fn; i++ {
		filePaths[i] = C.CString(files[i].Path)
		fileNames[i] = C.CString(files[i].Name)
	}

	var pP, pN, fP, fN **C.char
	if pn > 0 {
		pP = (**C.char)(unsafe.Pointer(&projPaths[0]))
		pN = (**C.char)(unsafe.Pointer(&projNames[0]))
	}
	if fn > 0 {
		fP = (**C.char)(unsafe.Pointer(&filePaths[0]))
		fN = (**C.char)(unsafe.Pointer(&fileNames[0]))
	}
	// setBishDockMenuC converts to NSStrings before dispatch_async, so freeing here is safe.
	C.setBishDockMenuC(pP, pN, C.int(pn), fP, fN, C.int(fn))

	for i := 0; i < pn; i++ {
		C.free(unsafe.Pointer(projPaths[i]))
		C.free(unsafe.Pointer(projNames[i]))
	}
	for i := 0; i < fn; i++ {
		C.free(unsafe.Pointer(filePaths[i]))
		C.free(unsafe.Pointer(fileNames[i]))
	}
}
