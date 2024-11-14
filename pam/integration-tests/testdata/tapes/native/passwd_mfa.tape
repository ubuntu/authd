Hide
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Sleep ${AUTHD_SLEEP_COMMAND}
Show

Hide
Type "user-mfa"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "2"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "r"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "1"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "goodpass"
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Hide
Type "r"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "1"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} * 1.2
Show

Hide
Type "r"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "1"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} * 1.2
Show

Hide
Type "authd2404"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "authd2404"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG}
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
