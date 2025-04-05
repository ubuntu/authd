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
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show

ClearTerminal

Hide
TypeInPrompt+Shell `export ${AUTHD_TEST_TAPE_SSH_USER_VAR}=${AUTHD_TEST_TAPE_LOWER_CASE_USERNAME}`
Enter
TypeInPrompt+Shell "clear"
Enter
Wait
Show

# Repeat the authentication with different casing...

Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "authd2404"
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show

# Repeat the authentication with different casing...

Hide
TypeInPrompt+Shell `export ${AUTHD_TEST_TAPE_SSH_USER_VAR}=${AUTHD_TEST_TAPE_MIXED_CASE_USERNAME}`
Enter
TypeInPrompt+Shell "clear"
Enter
Wait
Show

Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "authd2404"
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
