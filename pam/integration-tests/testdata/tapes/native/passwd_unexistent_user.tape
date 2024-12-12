Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:\n/
Show

Hide
TypeUsername "user-unexistent"
Enter
Wait+Prompt /Choose your provider/
Show

Hide
Type "2"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show
