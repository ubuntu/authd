Hide
Wait
Type "if [ -v AUTHD_PAM_CLI_TERM ]; then export TERM=${AUTHD_PAM_CLI_TERM}; fi"
Enter
Show

Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Prompt /Choose your provider/
Show

Hide
Type "2"
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "r"
Enter
Wait+Screen /${AUTHD_QRCODE_TAPE_ITEM}\. Use a ${AUTHD_QRCODE_TAPE_ITEM_NAME}/
Wait+Prompt /Choose your authentication method/
Show

Hide
Type "${AUTHD_QRCODE_TAPE_ITEM}"
Enter
Wait+Prompt /Choose action/
Show

Hide
Type "2"
Enter
Wait+Prompt /Choose action/
Show

Hide
Type "2"
Enter
Wait+Prompt /Choose action/
Show

Hide
Type "2"
Enter
Wait+Prompt /Choose action/
Show

Hide
Type "2"
Enter
Wait+Prompt /Choose action/
Show

Hide
Type "1"
Enter
Show

Hide
Wait@${AUTHD_SLEEP_EXAMPLE_BROKER_QRCODE_WAIT} * 1.2
Show
