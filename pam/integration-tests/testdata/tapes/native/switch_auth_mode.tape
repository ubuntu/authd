Hide
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK} force_native_client=true"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "user-integration-switch-mode"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG}
Show

Hide
Type "2"
Enter
Sleep ${AUTHD_SLEEP_LONG} / 2
Show

Hide
Type "r"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT} * 3
Show

Hide
Type "2"
Enter
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} / 3
Show

Hide
Type "r"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT} / 2
Show

Hide
Type "3"
Enter
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} / 3
Show

Hide
Type "r"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "4"
Enter
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} / 3
Show

Hide
Type "r"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "5"
Enter
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} / 3
Show

Hide
Type "r"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "6"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "r"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "7"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
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
Type "r"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "invalid-selection"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "-1"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "6"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "4242"
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
