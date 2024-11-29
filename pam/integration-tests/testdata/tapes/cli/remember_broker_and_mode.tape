Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-integration-remember-mode"
Wait /Username: user-integration-remember-mode$/
Show

Hide
Enter
Wait+Screen /Select your provider/
Wait+Screen /2. ExampleBroker/
Show

Hide
Type "2"
Wait+Prompt /Gimme your password/
Escape
Wait+Screen /Select your authentication method/
Wait+Screen /7\. Authentication code/
Show

Hide
Type "7"
Wait+Screen /Enter your one time credential:\n>\n/
Show

Hide
Type "temporary pass0"
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show

Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-integration-remember-mode"
Wait /Username: user-integration-remember-mode$/
Show

Hide
Enter
Wait+Screen /Enter your one time credential:\n>\n/
Show

Hide
Type "temporary pass0"
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
