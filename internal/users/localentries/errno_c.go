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
	"sync"
)

type errnoError C.int

func (errno errnoError) Error() string {
	return C.GoString(C.strerror(C.int(errno)))
}

const (
	errNoEnt errnoError = C.ENOENT
	errSrch  errnoError = C.ESRCH
	errBadf  errnoError = C.EBADF
	errPerm  errnoError = C.EPERM
)

// All these functions are expected to be called while this mutex is locked.
var errnoMutex sync.Mutex

func unsetErrno() {
	C.unset_errno()
}

func getErrno() error {
	if errno := C.get_errno(); errno != 0 {
		return errnoError(errno)
	}
	return nil
}
