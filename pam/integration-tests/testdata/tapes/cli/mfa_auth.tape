Hide
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK}"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Escape
Backspace
Type "user-mfa"
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
Escape
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "1"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "goodpass"
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Hide
Escape
Sleep ${AUTHD_SLEEP_LONG} / 2
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG}
Show

Hide
Sleep ${AUTHD_SLEEP_LONG} * 5
Show

Hide
Escape
Sleep ${AUTHD_SLEEP_LONG} / 2
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG}
Show

Hide
Sleep ${AUTHD_SLEEP_LONG} * 5
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
