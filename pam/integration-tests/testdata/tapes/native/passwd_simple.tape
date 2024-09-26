Hide
Type "./pam_authd passwd socket=${AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK} force_native_client=true"
Enter
Sleep 300ms
Show

Hide
Type "user1"
Sleep 300ms
Show

Hide
Enter
Sleep 300ms
Show

Hide
Type "2"
Enter
Sleep 300ms
Show

Hide
Type "goodpass"
Enter
Sleep 2s
Show

Hide
Type "authd2404"
Enter
Sleep 300ms
Show

Hide
Type "authd2404"
Sleep 300ms
Show

Hide
Enter
Sleep 2s
Show

Hide
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK} force_native_client=true"
Enter
Sleep 300ms
Show

Hide
Type "user1"
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

Sleep 300ms
