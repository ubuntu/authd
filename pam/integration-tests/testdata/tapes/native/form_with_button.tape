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
Wait+Screen /8\. Authentication code/
Wait+Prompt /Choose your authentication method/
Show

Hide
Type "8"
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
Wait+Prompt /Enter your one time credential/
Show

Hide
Type "temporary pass00"
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
