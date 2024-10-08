Hide
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Sleep ${AUTHD_SLEEP_COMMAND}
Show

Hide
Escape
Backspace
Type "user-integration-qrcode-static-regenerate"
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
Escape
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type "6"
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Tab
Sleep ${AUTHD_SLEEP_EXAMPLE_BROKER_QRCODE_WAIT} / 3
Show

Hide
Enter@1ms 500
Sleep ${AUTHD_SLEEP_LONG} * 15
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
