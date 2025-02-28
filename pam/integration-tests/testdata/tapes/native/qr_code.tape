Hide
Wait
Type "if [ -v AUTHD_PAM_CLI_TERM ]; then export TERM=${AUTHD_PAM_CLI_TERM}; fi"
Enter
Show

Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
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
Wait+Screen /${AUTHD_QRCODE_TAPE_ITEM}\. Use a ${AUTHD_QRCODE_TAPE_ITEM_NAME}/
Wait+Prompt /Choose your authentication method/
Show

Hide
TypeInPrompt "${AUTHD_QRCODE_TAPE_ITEM}"
Show

Hide
Enter
Wait+Prompt /Choose action/
Show

Hide
TypeInPrompt "2"
Show

Hide
Enter
Wait+Prompt /Choose action/
Show

Hide
TypeInPrompt "2"
Show

Hide
Enter
Wait+Prompt /Choose action/
Show

Hide
TypeInPrompt "2"
Show

Hide
Enter
Wait+Prompt /Choose action/
Show

Hide
TypeInPrompt "2"
Show

Hide
Enter
Wait+Prompt /Choose action/
Show

Hide
TypeInPrompt "1"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
