package localentries

// #include <stdlib.h>
// #include <pwd.h>
// #include <grp.h>
import "C"
import "sync"

// Passwd represents a passwd entry.
type Passwd struct {
	Name string
	UID  uint32
}

var getpwentMutex sync.Mutex

// GetPasswdEntries returns all passwd entries.
func GetPasswdEntries() []Passwd {
	// This function repeatedly calls getpwent, which iterates over the records in the passwd database.
	// Use a mutex to avoid that parallel calls to this function interfere with each other.
	getpwentMutex.Lock()
	defer getpwentMutex.Unlock()

	C.setpwent()
	defer C.endpwent()

	var entries []Passwd
	for {
		cPasswd := C.getpwent()
		if cPasswd == nil {
			break
		}

		entries = append(entries, Passwd{
			Name: C.GoString(cPasswd.pw_name),
			UID:  uint32(cPasswd.pw_uid),
		})
	}

	return entries
}

// Group represents a group entry.
type Group struct {
	Name string
	GID  uint32
}

var getgrentMutex sync.Mutex

// GetGroupEntries returns all group entries.
func GetGroupEntries() []Group {
	// This function repeatedly calls getgrent, which iterates over the records in the group database.
	// Use a mutex to avoid that parallel calls to this function interfere with each other.
	getgrentMutex.Lock()
	defer getgrentMutex.Unlock()


var getgrentMutex sync.Mutex

	var entries []Group
	for {
		cGroup := C.getgrent()
		if cGroup == nil {
			break
		}

		entries = append(entries, Group{
			Name: C.GoString(cGroup.gr_name),
			GID:  uint32(cGroup.gr_gid),
		})
	}

	return entries
}
