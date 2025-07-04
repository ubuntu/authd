// Package localentries provides functions to access the local user and group database.
package localentries

/*
#define _GNU_SOURCE

#include <stdlib.h>
#include <pwd.h>
#include <grp.h>
*/
import "C"

import (
	"errors"
	"runtime"
	"sync"
	"syscall"

	"github.com/ubuntu/decorate"
)

// Group represents a group entry.
type Group struct {
	Name   string
	GID    uint32
	Passwd string
}

var getgrentMu sync.Mutex

// GetGroupEntries returns all group entries.
func GetGroupEntries() (entries []Group, err error) {
	decorate.OnError(&err, "getgrent_r")

	// This function repeatedly calls getgrent_r, which iterates over the records in the group database.
	// Use a mutex to avoid that parallel calls to this function interfere with each other.
	// It would be nice to use fgetgrent_r, that is thread safe, but it can only
	// iterate over a stream, while we want to iterate over all the NSS sources too.
	getgrentMu.Lock()
	defer getgrentMu.Unlock()

	C.setgrent()
	defer C.endgrent()

	var group C.struct_group
	var groupPtr *C.struct_group
	buf := make([]C.char, 1024)

	pinner := runtime.Pinner{}
	defer pinner.Unpin()

	pinner.Pin(&group)
	pinner.Pin(&buf[0])

	for {
		ret := C.getgrent_r(&group, &buf[0], C.size_t(len(buf)), &groupPtr)
		errno := syscall.Errno(ret)

		if errors.Is(errno, syscall.ERANGE) {
			buf = make([]C.char, len(buf)*2)
			pinner.Pin(&buf[0])
			continue
		}
		if errors.Is(errno, syscall.ENOENT) {
			return entries, nil
		}
		if !errors.Is(errno, syscall.Errno(0)) {
			return nil, errno
		}

		entries = append(entries, Group{
			Name:   C.GoString(groupPtr.gr_name),
			Passwd: C.GoString(groupPtr.gr_passwd),
			GID:    uint32(groupPtr.gr_gid),
		})
	}
}
