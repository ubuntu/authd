Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-unexistent"
Wait /Username: user-unexistent$/
Show

Hide
Enter
Wait+Screen /Select your provider/
Wait+Screen /2. ExampleBroker/
Show

Hide
Type "2"
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show
