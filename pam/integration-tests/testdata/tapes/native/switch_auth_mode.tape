Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Screen /Choose your provider:\n>[\n]*$/
Show

Hide
Type "2"
Enter
Wait+Screen /Gimme your password:\n>[\n]*$/
Show

Hide
Type "r"
Enter
Wait+Screen /Choose your authentication method:\n>[\n]*$/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_SEND_URL_TO_EMAIL_ITEM}\. Send URL to /
Show

Hide
Type "${AUTHD_SWITCH_AUTH_MODE_TAPE_SEND_URL_TO_EMAIL_ITEM}"
Enter
Wait+Screen /enter the code:\n>[\n]*$/
Show

Hide
Type "r"
Enter
Wait+Screen /Choose your authentication method:\n>[\n]*$/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_FIDO_DEVICE_FOO_ITEM}\. Use your fido device foo/
Show

Hide
Type "${AUTHD_SWITCH_AUTH_MODE_TAPE_FIDO_DEVICE_FOO_ITEM}"
Enter
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Plug your fido device and press with your thumb:\n>[\n]*$/
Show

Hide
Type "r"
Enter
Wait+Screen /Choose your authentication method:\n>[\n]*$/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_PHONE_33_ITEM}\. Use your phone \+33\.\.\./
Show

Hide
Type "${AUTHD_SWITCH_AUTH_MODE_TAPE_PHONE_33_ITEM}"
Enter
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Unlock your phone \+33\.\.\. or accept request on web interface:/
Show

Hide
Type "r"
Enter
Wait+Screen /Choose your authentication method:\n>[\n]*$/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_PHONE_1_ITEM}\. Use your phone \+1\.\.\./
Show

Hide
Type "${AUTHD_SWITCH_AUTH_MODE_TAPE_PHONE_1_ITEM}"
Enter
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Unlock your phone \+1\.\.\. or accept request on web interface:\n>[\n]*$/
Show

Hide
Type "r"
Enter
Wait+Screen /Choose your authentication method:\n>[\n]*$/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_PIN_CODE_ITEM}\. Pin code/
Show

Hide
Type "${AUTHD_SWITCH_AUTH_MODE_TAPE_PIN_CODE_ITEM}"
Enter
Wait+Screen /Enter your pin code:\n>[\n]*$/
Show

Hide
Type "r"
Enter
Wait+Screen /Choose your authentication method:\n>[\n]*$/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_QR_OR_LOGIN_CODE_ITEM}\. Use a ${AUTHD_SWITCH_AUTH_MODE_TAPE_QR_OR_LOGIN_CODE_ITEM_NAME}/
Show

Hide
Type "${AUTHD_SWITCH_AUTH_MODE_TAPE_QR_OR_LOGIN_CODE_ITEM}"
Enter
Wait+Screen /Choose action:\n>[\n]*$/
Show

Hide
Type "r"
Enter
Wait+Screen /Choose your authentication method:\n>[\n]*$/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_AUTHENTICATION_CODE_ITEM}\. Authentication code/
Show

Hide
Type "${AUTHD_SWITCH_AUTH_MODE_TAPE_AUTHENTICATION_CODE_ITEM}"
Enter
Wait+Screen /Choose action:\n>[\n]*$/
Show

Hide
Type "r"
Enter
Wait+Screen /Choose your authentication method:\n>[\n]*$/
Show

Hide
Type "invalid-selection"
Enter
Wait+Screen /Choose your authentication method:\n>[\n]*$/
Show

Hide
Type "-1"
Enter
Wait+Screen /Choose your authentication method:\n>[\n]*$/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_PIN_CODE_ITEM}\. Pin code/
Show

Hide
Type "${AUTHD_SWITCH_AUTH_MODE_TAPE_PIN_CODE_ITEM}"
Enter
Wait+Screen /Enter your pin code:\n>[\n]*$/
Show

Hide
Type "4242"
Enter
Wait
Show
