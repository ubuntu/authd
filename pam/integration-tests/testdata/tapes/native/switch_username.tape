Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:\n/
Show

Hide
TypeUsername "user-integration-switch-username"
Show

Hide
Enter
Wait+Prompt /Choose your provider/
Show

Hide
TypeInPrompt "r"
Show

Hide
Enter
Wait /Username:\n/
Show

Hide
TypeUsername "user-integration-username-switched"
Show

Hide
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
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
