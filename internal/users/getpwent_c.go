package users

// #include <stdlib.h>
// #include <pwd.h>
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
