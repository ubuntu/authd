// Package localentries provides functions to access the local user and group database.
//
//nolint:dupl // This it not a duplicate of getpwent_c.go
package localentries

/*
#include <stdlib.h>
#include <pwd.h>
#include <grp.h>
#include <errno.h>

#cgo nocallback endgrent
#cgo nocallback getgrent
#cgo nocallback getgrnam
#cgo noescape getgrnam
#cgo nocallback setgrent
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ubuntu/authd/internal/errno"
)

// Group represents a group entry.
type Group struct {
	Name   string
	GID    uint32
	Passwd string
}

var getgrentMu sync.Mutex

func getGroupEntry() (*C.struct_group, error) {
	errno.Lock()
	defer errno.Unlock()

	cGroup := C.getgrent()
	if cGroup != nil {
		return cGroup, nil
	}

	err := errno.Get()
	// It's not documented in the man page, but apparently getgrent sets errno to ENOENT when there are no more
	// entries in the group database.
	if errors.Is(err, errno.ErrNoEnt) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getgrent: %v", err)
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
	errno.Lock()
	defer errno.Unlock()

	cGroup := C.getgrnam(C.CString(name))
	if cGroup == nil {
		err := errno.Get()
		if err == nil ||
			errors.Is(err, errno.ErrNoEnt) ||
			errors.Is(err, errno.ErrSrch) ||
			errors.Is(err, errno.ErrBadf) ||
			errors.Is(err, errno.ErrPerm) {
			return Group{}, ErrGroupNotFound
		}
		return Group{}, fmt.Errorf("getgrnam: %v", err)
	}

	return Group{
		Name: C.GoString(cGroup.gr_name),
		GID:  uint32(cGroup.gr_gid),
	}, nil
}
