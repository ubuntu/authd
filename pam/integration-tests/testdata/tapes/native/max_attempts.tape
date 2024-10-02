Hide
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK} force_native_client=true"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "user-integration-max-attempts"
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
Type "wrongpass"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
Show

Hide
Type "wrongpass"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
Show

Hide
Type "wrongpass"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
Show

Hide
Type "wrongpass"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
Show

Hide
Type "wrongpass"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
