Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Nth(1) /Username:/
Show

Hide
Type ""
Enter
Wait+Nth(2) /Username:/
Show

Hide
TypeUsername "user-integration-user-selection"
Enter
Wait+Prompt /Choose your provider/
Show

Hide
Type "2"
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "goodpass"
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
