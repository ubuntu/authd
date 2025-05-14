Hide
Type "if [ -v AUTHD_PAM_CLI_TERM ]; then export TERM=${AUTHD_PAM_CLI_TERM}; fi"
Enter
Type "clear"
Enter
Wait
Show

Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Screen /Select your provider/
Wait+Screen /2. ExampleBroker/
Show

Hide
Type "2"
Wait+CLIPrompt /Gimme your password/ /Press escape key to go back to select the authentication method/
Show

Hide
Escape
Wait+Screen /Select your authentication method/
Wait+Screen /6\. Use a QR code/
Show

Hide
Type "6"
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_QRCODE_WAIT}/3 /Scan the qrcode or enter the code in the login page/
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_QRCODE_WAIT}/3 /\b1337\b/
Show

Hide
Tab
Show

Hide
Sleep ${AUTHD_SLEEP_QRCODE_RESELECTION_WAIT}
Enter
Wait+Screen /Scan the qrcode or enter the code in the login page/
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_QRCODE_WAIT}/3 /\b1338\b/
Show

Hide
Sleep ${AUTHD_SLEEP_QRCODE_RESELECTION_WAIT}
Enter
Wait+Screen /Scan the qrcode or enter the code in the login page/
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_QRCODE_WAIT}/3 /\b1339\b/
Show

Hide
Sleep ${AUTHD_SLEEP_QRCODE_RESELECTION_WAIT}
Enter
Wait+Screen /Scan the qrcode or enter the code in the login page/
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_QRCODE_WAIT}/3 /\b1340\b/
Show

Hide
Sleep ${AUTHD_SLEEP_QRCODE_RESELECTION_WAIT}
Enter
Wait+Screen /Scan the qrcode or enter the code in the login page/
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_QRCODE_WAIT}/3 /\b1341\b/
Show

Hide
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
