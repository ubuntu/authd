Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND} invalid_flag=foo bar"
Enter
Wait /Username: user name\n/
Show

Hide
TypeUsername "user2"
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
