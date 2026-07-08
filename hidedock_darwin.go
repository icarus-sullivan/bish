package main

/*
// Forward declaration only — definition lives in hidedock_impl_darwin.go
void bishHideDockIconC(void);
*/
import "C"

func hideDockIcon() {
	C.bishHideDockIconC()
}
