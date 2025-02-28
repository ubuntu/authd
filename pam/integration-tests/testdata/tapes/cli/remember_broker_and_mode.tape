Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
TypeUsername "user-integration-remember-mode"
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
Wait+CLIPrompt /Enter your one time credential/ /\[ Resend SMS \(1 sent\) \]/
Show

Hide
TypeInPrompt "temporary pass0"
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show

ClearTerminal

Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
TypeUsername "user-integration-remember-mode"
Show

Hide
Enter
Wait+CLIPrompt /Enter your one time credential/ /\[ Resend SMS \(1 sent\) \]/
Show

Hide
TypeInPrompt "temporary pass0"
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
