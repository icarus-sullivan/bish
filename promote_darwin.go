package main

/*
// Forward declaration only — definition lives in promote_impl_darwin.go
void bishPromoteToRegularC(void);
*/
import "C"

func promoteToRegular() {
	C.bishPromoteToRegularC()
}
