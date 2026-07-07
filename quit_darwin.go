package main

/*
void installQuitInterceptC(void);
*/
import "C"

// goMarkQuitRequested is called from native code when the user chooses
// Quit (Cmd+Q / app menu), as opposed to closing this window individually.
//
//export goMarkQuitRequested
func goMarkQuitRequested() {
	if globalApp != nil {
		globalApp.SetQuitRequested()
	}
}

func installQuitIntercept() {
	C.installQuitInterceptC()
}
