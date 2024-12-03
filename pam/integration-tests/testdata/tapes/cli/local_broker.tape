Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-local-broker"
Wait /Username: user-local-broker$/
Show

Hide
Enter
Wait+Screen /Select your provider/
Wait+Screen /1. local/
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
