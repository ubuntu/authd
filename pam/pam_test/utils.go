// Package pam_test includes test tools for the PAM module
package pam_test

/*
#include <stdlib.h>
*/
import "C"
import (
	"unsafe"

	"github.com/msteinert/pam"
)

func allocateCBytes(bytes []byte) pam.BinaryPointer {
	if bytes == nil {
		return nil
	}
	return pam.BinaryPointer(C.CBytes(bytes))
}

func cBytesToBytes(ptr pam.BinaryPointer, size int) []byte {
	if ptr == nil {
		return nil
	}
	return C.GoBytes(unsafe.Pointer(ptr), C.int(size))
}

func releaseCBytesPointer(ptr pam.BinaryPointer) {
	C.free(unsafe.Pointer(ptr))
}
