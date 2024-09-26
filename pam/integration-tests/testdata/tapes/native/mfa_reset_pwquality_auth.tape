Hide
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK} force_native_client=true"
Enter
Sleep 300ms
Show

Hide
Type "user-mfa-with-reset"
Sleep 300ms
Show

Hide
Enter
Sleep 300ms
Show

Hide
Type "2"
Show

Hide
Enter
Sleep 300ms
Show

Hide
Type "goodpass"
Sleep 300ms
Show

Hide
Enter
Sleep 2s
Show

# Wait for MFA auth to happen
Hide
Enter
Sleep 5s
Show

Hide
Type "1"
Enter
Sleep 300ms
Show

Hide
Type "goodpass"
Enter
Sleep 1s
Show

Hide
Type "password"
Enter
Sleep 1s
Show

Hide
Type "goodpass"
Enter
Sleep 1s
Show

Hide
Type "1234"
Enter
Sleep 1s
Show

Hide
Type "authd2404"
Sleep 300ms
Show

Hide
Enter
Sleep 500ms
Show

Hide
Type "authd2404"
Sleep 500ms
Show

Hide
Enter
Sleep 2s
Show

Sleep 300ms
