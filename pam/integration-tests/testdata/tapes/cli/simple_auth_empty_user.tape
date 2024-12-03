Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-integration-not-empty"
Wait+Screen /Username: user-integration-not-empty\n/
Show

Hide
Backspace 26
Wait /Username: user name$/
Show

Hide
Enter
Wait /Username: user name$/
Show

Hide
Escape
Backspace
Type "user-integration-was-empty"
Wait+Screen /Username: user-integration-was-empty\n/
Show

Hide
Enter
Wait+Screen /Select your provider/
Wait+Screen /2. ExampleBroker/
Show

Hide
Type "2"
Wait+Screen /Gimme your password/
Wait
Show

Hide
Type "goodpass"
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
