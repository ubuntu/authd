Hide
Type "if [ -v AUTHD_PAM_CLI_TERM ]; then export TERM=${AUTHD_PAM_CLI_TERM}; fi"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK}"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT} * 3
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
Type "6"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Tab
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_DEFAULT} * 3
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_DEFAULT} * 3
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_DEFAULT} * 3
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG}
Show

Hide
Sleep ${AUTHD_SLEEP_LONG} * 5
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
