Hide
Type "./pam_authd passwd socket=${AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK}"
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
Show

Hide
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
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} * 1.2
Show

Hide
Escape
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} / 2
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG} / 2
Show

Hide
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} * 1.2
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
Sleep ${AUTHD_SLEEP_LONG}
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
