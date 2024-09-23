// Package localentries provides functions to access local passwd entries.
//
//nolint:dupl // This it not a duplicate of getgrent_c.go
package localentries

/*
#include <stdlib.h>
#include <pwd.h>
#include <grp.h>
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
	"fmt"
	"sync"
)

// Passwd represents a passwd entry.
type Passwd struct {
	Name  string
	UID   uint32
	Gecos string
}

var getpwentMu sync.Mutex

func getPasswdEntry() (*C.struct_passwd, error) {
	errnoMutex.Lock()
	defer errnoMutex.Unlock()
	C.unset_errno()
	cPasswd := C.getpwent()
	if cPasswd == nil {
		errno := C.get_errno()
		// It's not documented in the man page, but apparently getpwent sets errno to ENOENT when there are no more
		// entries in the passwd database.
		if errno == C.ENOENT {
			return nil, nil
		}
		if errno != 0 {
			return nil, fmt.Errorf("getpwent: %v", C.GoString(C.strerror(errno)))
		}
	}
	return cPasswd, nil
}

// GetPasswdEntries returns all passwd entries.
func GetPasswdEntries() ([]Passwd, error) {
	// This function repeatedly calls getpwent, which iterates over the records in the passwd database.
	// Use a mutex to avoid that parallel calls to this function interfere with each other.
	getpwentMu.Lock()
	defer getpwentMu.Unlock()

	C.setpwent()
	defer C.endpwent()

	var entries []Passwd
	for {
		cPasswd, err := getPasswdEntry()
		if err != nil {
			return nil, err
		}
		if cPasswd == nil {
			// No more entries in the passwd database.
			break
		}

		entries = append(entries, Passwd{
			Name:  C.GoString(cPasswd.pw_name),
			UID:   uint32(cPasswd.pw_uid),
			Gecos: C.GoString(cPasswd.pw_gecos),
		})
	}

	return entries, nil
}

// ErrUserNotFound is returned when a user is not found.
var ErrUserNotFound = errors.New("user not found")

// GetPasswdByName returns the user with the given name.
func GetPasswdByName(name string) (Passwd, error) {
	C.unset_errno()
	cPasswd := C.getpwnam(C.CString(name))
	if cPasswd == nil {
		errno := C.get_errno()
		switch errno {
		case 0, C.ENOENT, C.ESRCH, C.EBADF, C.EPERM:
			return Passwd{}, ErrUserNotFound
		default:
			return Passwd{}, fmt.Errorf("getpwnam: %v", C.GoString(C.strerror(errno)))
		}
	}

	return Passwd{
		Name: C.GoString(cPasswd.pw_name),
		UID:  uint32(cPasswd.pw_uid),
	}, nil
}
