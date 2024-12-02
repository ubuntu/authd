Hide
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Sleep ${AUTHD_SLEEP_COMMAND}
Show

Hide
Type "2"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "goodpass"
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Hide
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
Enter
Show

Hide
Type "1234"
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
Enter
Show

Hide
Type "12345678"
Sleep ${AUTHD_SLEEP_DEFAULT} * 3
Enter
Show

Hide
Type "authd2404"
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
Enter
Show

Hide
Type "123456789"
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
Enter
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Hide
Type "authd2404"
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
Enter
Show

Hide
Type "authd2404"
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show
