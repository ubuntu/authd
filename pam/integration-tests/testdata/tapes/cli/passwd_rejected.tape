Hide
Type "./pam_authd passwd socket=${AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK}"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Escape
Backspace
Type "user-integration-invalid-new-password"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "2"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "goodpass"
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Hide
Type "noble2404"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Type "noble2404"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Hide
Type "authd2404"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Type "authd2404"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Sleep ${AUTHD_SLEEP_LONG}

# Repeat again, to check that we can use still use another new password

Hide
Type "./pam_authd passwd socket=${AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK}"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Escape
Backspace
Type "user-integration-invalid-new-password"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "authd2404"
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Hide
Type "noble2404"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Type "noble2404"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Hide
Type "goodpass"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Type "goodpass"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
