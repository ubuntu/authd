Hide
Type "./pam_authd passwd socket=${AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK} force_native_client=true"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "user-integration-bad-password"
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
Type "goodpass"
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Hide
Sleep ${AUTHD_SLEEP_DEFAULT}
Enter
Show

Hide
Type "1234"
Sleep ${AUTHD_SLEEP_DEFAULT}
Enter
Show

Hide
Type "12345678"
Sleep ${AUTHD_SLEEP_DEFAULT}
Enter
Show

Hide
Type "authd2404"
Sleep ${AUTHD_SLEEP_DEFAULT}
Enter
Show

Hide
Type "123456789"
Sleep ${AUTHD_SLEEP_DEFAULT}
Enter
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Hide
Type "authd2404"
Sleep ${AUTHD_SLEEP_DEFAULT}
Enter
Show

Hide
Type "authd2404"
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
