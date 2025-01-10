package localentries

/*
#include <errno.h>
#include <string.h>

static void unset_errno(void) {
  errno = 0;
}

static int get_errno(void) {
  return errno;
}
*/
import "C"

import (
	"errors"
	"sync"
)

// All these functions are expected to be called while this mutex is locked.
var errnoMutex sync.Mutex

func unsetErrno() {
	C.unset_errno()
}

func getErrno() C.int {
	return C.get_errno()
}

func errnoToError(errno C.int) error {
	return errors.New(C.GoString(C.strerror(errno)))
}
