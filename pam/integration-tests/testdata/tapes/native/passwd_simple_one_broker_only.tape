Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "goodpass"
Enter
Wait+Prompt /Enter your new password/
Show

Hide
Type "authd2404"
Enter
Wait+Prompt /Confirm Password/
Show

Hide
Type "authd2404"
Enter
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show

ClearTerminal

Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_LOGIN_COMMAND}"
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "authd2404"
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
