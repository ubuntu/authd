Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
TypeUsername "user-integration-sigint"
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
Ctrl+C
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show
