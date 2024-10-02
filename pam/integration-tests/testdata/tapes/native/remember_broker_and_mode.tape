Hide
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK} force_native_client=true"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "user-integration-remember-mode"
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
Type "8"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "1"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "temporary pass0"
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Hide
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK} force_native_client=true"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "user-integration-remember-mode"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "1"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "temporary pass0"
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
