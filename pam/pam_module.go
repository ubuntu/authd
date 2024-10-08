// Code generated by "pam-moduler -libname pam_authd.so -type pamModule -no-main -tags !pam_debug"; DO NOT EDIT.

//go:build !pam_debug

//go:generate go build "-ldflags=-extldflags -Wl,-soname,pam_authd.so" -buildmode=c-shared -o pam_authd.so -tags go_pam_module

// Package main is the package for the PAM module library.
package main

/*
#cgo LDFLAGS: -lpam -fPIC
#include <security/pam_modules.h>

typedef const char _const_char_t;
*/
import "C"

import (
	"errors"
	"fmt"
	"github.com/msteinert/pam/v2"
	"os"
	"unsafe"
)

var pamModuleHandler pam.ModuleHandler = &pamModule{}

// sliceFromArgv returns a slice of strings given to the PAM module.
func sliceFromArgv(argc C.int, argv **C._const_char_t) []string {
	r := make([]string, 0, argc)
	for _, s := range unsafe.Slice(argv, argc) {
		r = append(r, C.GoString(s))
	}
	return r
}

// handlePamCall is the function that translates C pam requests to Go.
func handlePamCall(pamh *C.pam_handle_t, flags C.int, argc C.int,
	argv **C._const_char_t, moduleFunc pam.ModuleHandlerFunc) C.int {
	if pamModuleHandler == nil {
		return C.int(pam.ErrNoModuleData)
	}

	if moduleFunc == nil {
		return C.int(pam.ErrIgnore)
	}

	mt := pam.NewModuleTransactionInvoker(pam.NativeHandle(pamh))
	err := mt.InvokeHandler(moduleFunc, pam.Flags(flags),
		sliceFromArgv(argc, argv))
	if err == nil {
		return 0
	}

	if (pam.Flags(flags)&pam.Silent) == 0 && !errors.Is(err, pam.ErrIgnore) {
		fmt.Fprintf(os.Stderr, "module returned error: %v\n", err)
	}

	var pamErr pam.Error
	if errors.As(err, &pamErr) {
		return C.int(pamErr)
	}

	return C.int(pam.ErrSystem)
}

//export pam_sm_authenticate
func pam_sm_authenticate(pamh *C.pam_handle_t, flags C.int, argc C.int, argv **C._const_char_t) C.int {
	return handlePamCall(pamh, flags, argc, argv, pamModuleHandler.Authenticate)
}

//export pam_sm_setcred
func pam_sm_setcred(pamh *C.pam_handle_t, flags C.int, argc C.int, argv **C._const_char_t) C.int {
	return handlePamCall(pamh, flags, argc, argv, pamModuleHandler.SetCred)
}

//export pam_sm_acct_mgmt
func pam_sm_acct_mgmt(pamh *C.pam_handle_t, flags C.int, argc C.int, argv **C._const_char_t) C.int {
	return handlePamCall(pamh, flags, argc, argv, pamModuleHandler.AcctMgmt)
}

//export pam_sm_open_session
func pam_sm_open_session(pamh *C.pam_handle_t, flags C.int, argc C.int, argv **C._const_char_t) C.int {
	return handlePamCall(pamh, flags, argc, argv, pamModuleHandler.OpenSession)
}

//export pam_sm_close_session
func pam_sm_close_session(pamh *C.pam_handle_t, flags C.int, argc C.int, argv **C._const_char_t) C.int {
	return handlePamCall(pamh, flags, argc, argv, pamModuleHandler.CloseSession)
}

//export pam_sm_chauthtok
func pam_sm_chauthtok(pamh *C.pam_handle_t, flags C.int, argc C.int, argv **C._const_char_t) C.int {
	return handlePamCall(pamh, flags, argc, argv, pamModuleHandler.ChangeAuthTok)
}
