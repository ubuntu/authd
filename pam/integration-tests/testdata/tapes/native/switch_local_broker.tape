Hide
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK} force_native_client=true"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "user-integration-switch-broker"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "2"
Enter
Sleep ${AUTHD_SLEEP_LONG} / 2
Show

Hide
Type "r"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "r"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "invalid-ID"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "555"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "1"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT} * 3
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
