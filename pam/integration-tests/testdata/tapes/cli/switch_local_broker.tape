Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-integration-switch-broker"
Wait /Username: user-integration-switch-broker$/
Show

Hide
Enter
Wait+Screen /Select your provider/
Wait+Screen /2. ExampleBroker/
Show

Hide
Type "2"
Wait+Prompt /Gimme your password/
Show

Hide
Escape
Wait+Screen /Select your authentication method/
Show

Hide
Escape
Wait+Screen /Select your provider/
Wait+Screen /1. local/
Show

Hide
Type "1"
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
