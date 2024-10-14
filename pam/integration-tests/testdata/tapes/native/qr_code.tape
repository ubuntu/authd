Hide
Type "if [ -v AUTHD_PAM_CLI_TERM ]; then export TERM=${AUTHD_PAM_CLI_TERM}; fi"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK} force_native_client=true"
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
Type "${AUTHD_QRCODE_TAPE_ITEM}"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "2"
Enter
Sleep ${AUTHD_SLEEP_LONG} / 2
Show

Hide
Type "2"
Enter
Sleep ${AUTHD_SLEEP_LONG} / 2
Show

Hide
Type "2"
Enter
Sleep ${AUTHD_SLEEP_LONG} / 2
Show

Hide
Type "2"
Enter
Sleep ${AUTHD_SLEEP_LONG} / 2
Show

Hide
Type "1"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_QRCODE_WAIT} * 1.2
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
