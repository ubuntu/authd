Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:\n/
Show

Hide
TypeUsername "r"
Show

Hide
Enter
Wait+Prompt /Choose your provider/
Show

Hide
Type "2"
Show

Hide
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
