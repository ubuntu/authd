Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Escape
Type "user-integration-switch-username"
Wait /Username: user-integration-switch-username$/
Show

Hide
Enter
Wait+Screen /Select your provider/
Show

Hide
Escape
Wait /Username: user-integration-switch-username$/
Backspace 15
Wait /Username: user-integration-$/
Type "username-switched"
Wait /Username: user-integration-username-switched$/
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
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
