package main

/*
#include <security/pam_appl.h>
#include <security/pam_modules.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

char *string_from_argv(int i, char **argv) {
  return strdup(argv[i]);
}

char *get_user(pam_handle_t *pamh) {
  if (!pamh)
    return NULL;
  int pam_err = 0;
  const char *user;
  if ((pam_err = pam_get_item(pamh, PAM_USER, (const void**)&user)) != PAM_SUCCESS)
    return NULL;
  return strdup(user);
}

char *set_user(pam_handle_t *pamh, char *username) {
  if (!pamh)
    return NULL;
  int pam_err = 0;
  if ((pam_err = pam_set_item(pamh, PAM_USER, (const void*)username)) != PAM_SUCCESS)
    return NULL;
  return NULL;
}
*/
import "C"

import (
	"unsafe"
)

// pamHandle allows to pass C.pam_handle_t to this package.
type pamHandle = *C.pam_handle_t

func sliceFromArgv(argc C.int, argv **C.char) []string {
	r := make([]string, 0, argc)
	for i := 0; i < int(argc); i++ {
		s := C.string_from_argv(C.int(i), argv)
		defer C.free(unsafe.Pointer(s))
		r = append(r, C.GoString(s))
	}
	return r
}

func getPAMUser(pamh *C.pam_handle_t) string {
	cUsername := C.get_user(pamh)
	if cUsername == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cUsername))
	return C.GoString(cUsername)
}

func setPAMUser(pamh *C.pam_handle_t, username string) {
	cUsername := C.CString(username)
	defer C.free(unsafe.Pointer(cUsername))

	C.set_user(pamh, cUsername)
}
