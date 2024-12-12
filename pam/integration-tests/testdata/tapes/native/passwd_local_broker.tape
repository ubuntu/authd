Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:\n/
Show

Hide
TypeUsername "user-local-broker"
Enter
Wait+Prompt /Choose your provider/
Show

Hide
Type "1"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show
