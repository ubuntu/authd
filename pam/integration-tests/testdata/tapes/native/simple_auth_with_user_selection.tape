Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Nth(1) /Username:/
Show

Hide
Type ""
Enter
Wait+Nth(2) /Username:/
Show

Hide
TypeUsername "${AUTHD_TEST_TAPE_USERNAME}"
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
