Hide
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Sleep ${AUTHD_SLEEP_COMMAND}
Show

Hide
Type "user-integration-not-empty"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Backspace 26
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Escape
Backspace
Type "user-integration-was-empty"
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
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
