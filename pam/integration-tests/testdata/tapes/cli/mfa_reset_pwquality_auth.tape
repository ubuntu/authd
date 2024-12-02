Hide
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Sleep ${AUTHD_SLEEP_COMMAND}
Show

Hide
Escape
Backspace
Type "user-mfa-with-reset"
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
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} / 2
Show

# Wait for MFA auth to happen
Hide
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} * 1.2
Show

Hide
Type "goodpass"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG}
Show

Hide
Type "password"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG}
Show

Hide
Type "goodpass"
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG}
Show

Hide
Type "1234"
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG}
Show

Hide
Type "authd2404"
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG} / 2
Show

Hide
Type "authd2404"
Sleep ${AUTHD_SLEEP_LONG} / 2
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show
