Hide
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK}"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Escape
Backspace
Type "user-integration-switch-mode"
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
Type "2"
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} / 3
Show

Hide
Escape
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "3"
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} / 3
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
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
