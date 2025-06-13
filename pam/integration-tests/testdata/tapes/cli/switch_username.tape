Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
Escape
TypeUsername "user-integration-switch-username"
Show

Hide
Enter
Wait+Screen /Select your provider/
Show

Hide
Escape
Wait /Username: user-integration-switch-username\n/
Backspace 15
Wait /Username: user-integration-\n/
Type "username-switched"
Wait /Username: user-integration-username-switched\n/
Show

Hide
Enter
Wait+Screen /Select your provider/
Wait+Screen /2. ExampleBroker/
Show

Hide
Type "2"
Wait+CLIPrompt /Gimme your password/ /Press escape key to go back to select the authentication method/
Show

Hide
TypeCLIPassword "goodpass"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
