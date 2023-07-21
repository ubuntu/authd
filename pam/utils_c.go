package main

/*
#include "pam-utils.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// pamHandle allows to pass C.pam_handle_t to this package.
type pamHandle = *C.pam_handle_t

type PamPrompt int

const (
	PamPromptEchoOff PamPrompt = 1
	PamPromptEchoOn  PamPrompt = 2
	PamPromptError   PamPrompt = 3
	PamPromptInfo    PamPrompt = 4
	PamPromptRadio   PamPrompt = 5
	PamPromptBinary  PamPrompt = 7
)

func sliceFromArgv(argc C.int, argv **C.char) []string {
	r := make([]string, 0, argc)
	for i := 0; i < int(argc); i++ {
		s := C.argv_string_get(argv, C.uint(i))
		defer C.free(unsafe.Pointer(s))
		r = append(r, C.GoString(s))
	}
	return r
}

func getUser(pamh *C.pam_handle_t, prompt string) (string, error) {
	cPrompt := C.CString(prompt)
	defer C.free(unsafe.Pointer(cPrompt))
	cUsername := C.get_user(pamh, cPrompt)
	if cUsername == nil {
		return "", fmt.Errorf("no user found")
	}
	defer C.free(unsafe.Pointer(cUsername))
	return C.GoString(cUsername), nil
}

func getPassword(prompt string) (string, error) {
	cPrompt := C.CString(prompt)
	defer C.free(unsafe.Pointer(cPrompt))
	cPasswd := C.getpass(cPrompt)
	if cPasswd == nil {
		return "", fmt.Errorf("no password found")
	}
	defer C.free(unsafe.Pointer(cPasswd))
	return C.GoString(cPasswd), nil
}

func getModuleName(pamh pamHandle) (string, error) {
	cModuleName := C.get_module_name(pamh)
	if cModuleName == nil {
		return "", fmt.Errorf("no module name found")
	}
	return C.GoString(cModuleName), nil
}

func pamConv(pamh pamHandle, prompt string, kind PamPrompt) (string, error) {
	cPrompt := C.CString(prompt)
	defer C.free(unsafe.Pointer(cPrompt))
	cResponse := C.send_msg(pamh, cPrompt, C.int(kind))
	if cResponse == nil {
		return "", fmt.Errorf("conversation with PAM application failed")
	}
	defer C.free(unsafe.Pointer(cResponse))
	return C.GoString(cResponse.resp), nil
}

func sendInfo(pamh pamHandle, prompt string) error {
	_, err := pamConv(pamh, "INFO: "+prompt, PamPromptInfo)
	return err
}

func sendInfof(pamh pamHandle, format string, args ...interface{}) error {
	return sendInfo(pamh, fmt.Sprintf(format, args...))
}

func sendError(pamh pamHandle, prompt string) error {
	_, err := pamConv(pamh, "ERROR: "+prompt, PamPromptError)
	return err
}

func sendErrorf(pamh pamHandle, format string, args ...interface{}) error {
	return sendInfo(pamh, fmt.Sprintf(format, args...))
}

func requestInput(pamh pamHandle, prompt string) (string, error) {
	return pamConv(pamh, prompt+": ", PamPromptEchoOn)
}

func requestSecret(pamh pamHandle, prompt string) (string, error) {
	return pamConv(pamh, prompt+": ", PamPromptEchoOff)
}
