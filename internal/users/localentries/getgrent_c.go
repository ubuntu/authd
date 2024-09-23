// Package localentries provides functions to access the local user and group database.
//
//nolint:dupl // This it not a duplicate of getpwent_c.go
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

// Group represents a group entry.
type Group struct {
	Name   string
	GID    uint32
	Passwd string
}

var getgrentMu sync.Mutex

func getGroupEntry() (*C.struct_group, error) {
	errnoMutex.Lock()
	defer errnoMutex.Unlock()

	C.unset_errno()
	cGroup := C.getgrent()
	if cGroup == nil {
		errno := C.get_errno()
		// It's not documented in the man page, but apparently getgrent sets errno to ENOENT when there are no more
		// entries in the group database.
		if errno == C.ENOENT {
			return nil, nil
		}
		if errno != 0 {
			return nil, fmt.Errorf("getgrent: %v", C.GoString(C.strerror(errno)))
		}
	}
	return cGroup, nil
}

// GetGroupEntries returns all group entries.
func GetGroupEntries() ([]Group, error) {
	// This function repeatedly calls getgrent, which iterates over the records in the group database.
	// Use a mutex to avoid that parallel calls to this function interfere with each other.
	getgrentMu.Lock()
	defer getgrentMu.Unlock()

	C.setgrent()
	defer C.endgrent()

	var entries []Group
	for {
		cGroup, err := getGroupEntry()
		if err != nil {
			return nil, err
		}
		if cGroup == nil {
			// No more entries in the group database.
			break
		}

		entries = append(entries, Group{
			Name:   C.GoString(cGroup.gr_name),
			GID:    uint32(cGroup.gr_gid),
			Passwd: C.GoString(cGroup.gr_passwd),
		})
	}

	return entries, nil
}

// ErrGroupNotFound is returned when a group is not found.
var ErrGroupNotFound = errors.New("group not found")

// GetGroupByName returns the group with the given name.
func GetGroupByName(name string) (Group, error) {
	errnoMutex.Lock()
	defer errnoMutex.Unlock()

	C.unset_errno()
	cGroup := C.getgrnam(C.CString(name))
	if cGroup == nil {
		errno := C.get_errno()
		switch errno {
		case 0, C.ENOENT, C.ESRCH, C.EBADF, C.EPERM:
			return Group{}, ErrGroupNotFound
		default:
			return Group{}, fmt.Errorf("getgrnam: %v", C.GoString(C.strerror(errno)))
		}
	}

	return Group{
		Name: C.GoString(cGroup.gr_name),
		GID:  uint32(cGroup.gr_gid),
	}, nil
}
