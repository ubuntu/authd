Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:\n/
Show

Hide
TypeUsername "${AUTHD_TEST_TAPE_USERNAME}"
Enter
Wait+Prompt /Choose your provider/
Show

Hide
TypeInPrompt "1"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show
