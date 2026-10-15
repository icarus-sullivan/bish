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

//export goNewWindowDock
func goNewWindowDock() {
	if globalApp != nil {
		go globalApp.NewWindow() //nolint
	}
}

//export goNewFileDock
func goNewFileDock() {
	if globalApp != nil {
		go globalApp.NewFileWindow() //nolint
	}
}

func setBishDockMenuFromRecents(projects []*project.RecentEntry, files []*project.RecentFile) {
	pn := len(projects)
	if pn > 8 {
		pn = 8
	}
	fn := len(files)
	// macOS defaults to ten recent documents. Prefer project folders, as VS
	// Code does for workspaces, and use any remaining slots for files.
	if fn > 10-pn {
		fn = 10 - pn
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
