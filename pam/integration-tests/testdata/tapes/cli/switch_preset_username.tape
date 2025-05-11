Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Screen /Select your provider/
Wait+Screen /2. ExampleBroker/
Show

Hide
Escape
Show

Hide
Type "2"
Wait+CLIPrompt /Gimme your password/ /Press escape key to go back to select the authentication method/
Show

Hide
Escape
Wait+Screen /Select your authentication method/
Show

Hide
Escape
Sleep ${AUTHD_SLEEP_DEFAULT}
Wait+Screen /Select your provider/
Wait+Screen /2. ExampleBroker/
Show

Hide
Escape 10
Sleep ${AUTHD_SLEEP_DEFAULT} * 2
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
