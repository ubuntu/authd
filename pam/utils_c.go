package main

/*
#cgo pkg-config: gdm-pam-extensions
#include "pam-utils.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/elliotchance/orderedmap/v2"
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

func gdmChoiceListSupported() bool {
	return C.gdm_choices_list_supported() != C.bool(false)
}

func gdmChoiceListRequest(pamh pamHandle, prompt string,
	choices *orderedmap.OrderedMap[string, string]) (string, error) {
	if choices.Len() == 0 {
		return "", errors.New("no choices provided")
	}

	cPrompt := C.CString(prompt)
	defer C.free(unsafe.Pointer(cPrompt))
	cChoicesRequest := C.gdm_choices_request_create(cPrompt, C.ulong(choices.Len()))
	defer C.gdm_choices_request_free(cChoicesRequest)

	var i = 0
	for el := choices.Front(); el != nil; el = el.Next() {
		cKey := C.CString(el.Key)
		defer C.free(unsafe.Pointer(cKey))
		cText := C.CString(el.Value)
		defer C.free(unsafe.Pointer(cText))

		C.gdm_choices_request_set(cChoicesRequest, C.ulong(i), cKey, cText)
		i++
	}

	cReply := C.gdm_choices_request_ask(pamh, cChoicesRequest)
	defer C.free(unsafe.Pointer(cReply))
	if cReply == nil {
		return "", errors.New("GDM didn't return any choice")
	}

	reply := C.GoString(cReply)
	if _, ok := choices.Get(reply); ok {
		return reply, nil
	}

	return "", fmt.Errorf("reply %s is not known", reply)
}
