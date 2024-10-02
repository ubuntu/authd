package localgroups

// #include <stdlib.h>
// #include <pwd.h>
// #include <grp.h>
// #include <errno.h>
// #include <string.h>
//
// void unset_errno(void) {
//   errno = 0;
// }
//
// int get_errno(void) {
//   return errno;
// }
import "C"
import "fmt"

// getPasswdUsernames gets the list of users using `getpwent` and returns their usernames.
func getPasswdUsernames() ([]string, error) {
	C.setpwent()
	defer C.endpwent()

	var entries []string
	for {
		C.unset_errno()
		cPasswd := C.getpwent()
		if cPasswd == nil && C.get_errno() != 0 {
			return nil, fmt.Errorf("getpwent failed: %v", C.GoString(C.strerror(C.get_errno())))
		}
		if cPasswd == nil {
			break
		}

		entries = append(entries, C.GoString(cPasswd.pw_name))
	}

	return entries, nil
}

// Passwd represents a passwd entry.
type Passwd struct {
	Name string
	UID  uint32
}

// GetPasswdEntries returns all passwd entries.
func GetPasswdEntries() ([]Passwd, error) {
	C.setpwent()
	defer C.endpwent()

	var entries []Passwd
	for {
		C.unset_errno()
		cPasswd := C.getpwent()
		if cPasswd == nil && C.get_errno() != 0 {
			return nil, fmt.Errorf("getpwent failed: %v", C.GoString(C.strerror(C.get_errno())))
		}
		if cPasswd == nil {
			break
		}

		entries = append(entries, Passwd{
			Name: C.GoString(cPasswd.pw_name),
			UID:  uint32(cPasswd.pw_uid),
		})
	}

	return entries, nil
}

// Group represents a group entry.
type Group struct {
	Name string
	GID  uint32
}

// GetGroupEntries returns all group entries.
func GetGroupEntries() ([]Group, error) {
	C.setgrent()
	defer C.endgrent()

	var entries []Group
	for {
		C.unset_errno()
		cGroup := C.getgrent()
		if cGroup == nil && C.get_errno() != 0 {
			return nil, fmt.Errorf("getgrent failed: %v", C.GoString(C.strerror(C.get_errno())))
		}

		if cGroup == nil {
			break
		}

		entries = append(entries, Group{
			Name: C.GoString(cGroup.gr_name),
			GID:  uint32(cGroup.gr_gid),
		})
	}

	return entries, nil
}
