Hide
Type "./pam_authd passwd socket=${AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK}"
Enter
Sleep 300ms
Show

Hide
Escape
Backspace
Type "user-integration-invalid-new-password"
Sleep 300ms
Show

Hide
Enter
Sleep 300ms
Show

Hide
Type "2"
Sleep 300ms
Show

Hide
Type "goodpass"
Enter
Sleep 2s
Show

Hide
Type "noble2404"
Sleep 300ms
Show

Hide
Enter
Type "noble2404"
Sleep 300ms
Show

Hide
Enter
Sleep 2s
Show

Hide
Type "authd2404"
Sleep 300ms
Show

Hide
Enter
Type "authd2404"
Sleep 300ms
Show

Hide
Enter
Sleep 2s
Show

Sleep 1s

# Repeat again, to check that we can use still use another new password

Hide
Type "./pam_authd passwd socket=${AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK}"
Enter
Sleep 300ms
Show

Hide
Escape
Backspace
Type "user-integration-invalid-new-password"
Sleep 300ms
Show

Hide
Enter
Sleep 300ms
Show

Hide
Type "authd2404"
Enter
Sleep 2s
Show

Hide
Type "noble2404"
Sleep 300ms
Show

Hide
Enter
Type "noble2404"
Sleep 300ms
Show

Hide
Enter
Sleep 2s
Show

Hide
Type "goodpass"
Sleep 300ms
Show

Hide
Enter
Type "goodpass"
Sleep 300ms
Show

Hide
Enter
Sleep 2s
Show

Sleep 300ms
