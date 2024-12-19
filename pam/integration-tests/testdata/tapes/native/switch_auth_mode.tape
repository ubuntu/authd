Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Prompt /Choose your provider/
Show

Hide
TypeInPrompt "2"
Show

Hide
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "r"
Show

Hide
Enter
Wait+Prompt /Choose your authentication method/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_SEND_URL_TO_EMAIL_ITEM}\. Send URL to /
Show

Hide
TypeInPrompt "${AUTHD_SWITCH_AUTH_MODE_TAPE_SEND_URL_TO_EMAIL_ITEM}"
Show

Hide
Enter
Wait+Prompt /enter the code/
Show

Hide
TypeInPrompt "r"
Show

Hide
Enter
Wait+Prompt /Choose your authentication method/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_FIDO_DEVICE_FOO_ITEM}\. Use your fido device foo/
Show

Hide
TypeInPrompt "${AUTHD_SWITCH_AUTH_MODE_TAPE_FIDO_DEVICE_FOO_ITEM}"
Show

Hide
Enter
Wait+Prompt@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Plug your fido device and press with your thumb/
Show

Hide
TypeInPrompt "r"
Show

Hide
Enter
Wait+Prompt /Choose your authentication method/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_PHONE_33_ITEM}\. Use your phone \+33\.\.\./
Show

Hide
TypeInPrompt "${AUTHD_SWITCH_AUTH_MODE_TAPE_PHONE_33_ITEM}"
Show

Hide
Enter
Wait+Prompt@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Unlock your phone \+33\.\.\. or accept request on web interface/
Show

Hide
TypeInPrompt "r"
Show

Hide
Enter
Wait+Prompt /Choose your authentication method/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_PHONE_1_ITEM}\. Use your phone \+1\.\.\./
Show

Hide
TypeInPrompt "${AUTHD_SWITCH_AUTH_MODE_TAPE_PHONE_1_ITEM}"
Show

Hide
Enter
Wait+Prompt@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Unlock your phone \+1\.\.\. or accept request on web interface/
Show

Hide
TypeInPrompt "r"
Show

Hide
Enter
Wait+Prompt /Choose your authentication method/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_PIN_CODE_ITEM}\. Pin code/
Show

Hide
TypeInPrompt "${AUTHD_SWITCH_AUTH_MODE_TAPE_PIN_CODE_ITEM}"
Show

Hide
Enter
Wait+Prompt /Enter your pin code/
Show

Hide
Type "r"
Show

Hide
Enter
Wait+Prompt /Choose your authentication method/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_QR_OR_LOGIN_CODE_ITEM}\. Use a ${AUTHD_SWITCH_AUTH_MODE_TAPE_QR_OR_LOGIN_CODE_ITEM_NAME}/
Show

Hide
TypeInPrompt "${AUTHD_SWITCH_AUTH_MODE_TAPE_QR_OR_LOGIN_CODE_ITEM}"
Show

Hide
Enter
Wait+Prompt /Choose action/
Show

Hide
TypeInPrompt "r"
Show

Hide
Enter
Wait+Prompt /Choose your authentication method/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_AUTHENTICATION_CODE_ITEM}\. Authentication code/
Show

Hide
TypeInPrompt "${AUTHD_SWITCH_AUTH_MODE_TAPE_AUTHENTICATION_CODE_ITEM}"
Show

Hide
Enter
Wait+Prompt /Choose action/
Show

Hide
TypeInPrompt "r"
Show

Hide
Enter
Wait+Prompt /Choose your authentication method/
Show

Hide
TypeInPrompt "invalid-selection"
Show

Hide
Enter
Wait+Prompt /Choose your authentication method/
Show

Hide
TypeInPrompt "-1"
Show

Hide
Enter
Wait+Prompt /Choose your authentication method/
Wait+Screen /${AUTHD_SWITCH_AUTH_MODE_TAPE_PIN_CODE_ITEM}\. Pin code/
Show

Hide
TypeInPrompt "${AUTHD_SWITCH_AUTH_MODE_TAPE_PIN_CODE_ITEM}"
Show

Hide
Enter
Wait+Prompt /Enter your pin code/
Show

Hide
TypeInPrompt "4242"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
