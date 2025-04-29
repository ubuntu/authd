package userslocking

/*
#include <shadow.h>
*/
import "C"

import (
	"fmt"

	"github.com/ubuntu/authd/internal/errno"
)

// writeLock is the default locking implementation.
func writeLock() error {
	errno.Lock()
	defer errno.Unlock()

	if C.lckpwdf() == 0 {
		return nil
	}

	if err := errno.Get(); err != nil {
		return fmt.Errorf("%w: %w", ErrLock, err)
	}

	return ErrLock
}

// writeUnlock is the default unlocking implementation.
func writeUnlock() error {
	errno.Lock()
	defer errno.Unlock()

	if C.ulckpwdf() == 0 {
		return nil
	}

	if err := errno.Get(); err != nil {
		return fmt.Errorf("%w: %w", ErrUnlock, err)
	}

	return ErrUnlock
}
