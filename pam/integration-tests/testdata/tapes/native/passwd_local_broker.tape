Hide
Type "./pam_authd passwd socket=${AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK} force_native_client=true"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "user-integration-native-passwd-local-broker"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "1"
Enter
Sleep ${AUTHD_SLEEP_LONG}
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
