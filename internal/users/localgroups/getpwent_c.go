package localgroups

// #include <stdlib.h>
// #include <pwd.h>
// #include <grp.h>
import "C"

// getPasswdUsernames gets the list of users using `getpwent` and returns their usernames.
func getPasswdUsernames() []string {
	C.setpwent()
	defer C.endpwent()

	var entries []string
	for {
		cPasswd := C.getpwent()
		if cPasswd == nil {
			break
		}

		entries = append(entries, C.GoString(cPasswd.pw_name))
	}

	return entries
}

// Passwd represents a passwd entry.
type Passwd struct {
	Name string
	UID  uint32
}

// GetPasswdEntries returns all passwd entries.
func GetPasswdEntries() []Passwd {
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

// GetGroupEntries returns all group entries.
func GetGroupEntries() []Group {
	C.setgrent()
	defer C.endgrent()

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
