Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
TypeUsername "user-integration-not-empty"
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
TypeUsername "user-integration-was-empty"
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
TypeCLIPassword "goodpass"
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
