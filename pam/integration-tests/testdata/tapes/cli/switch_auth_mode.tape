Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
TypeUsername "user-integration-switch-mode"
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
Wait+Screen /2\. Send URL to /
Show

Hide
Type "2"
Wait+Prompt@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/3 /Click on the link received at .* or enter the code/
Show

Hide
Escape
Wait+Screen /Select your authentication method/
Wait+Screen /3\. Use your fido device foo/
Show

Hide
Type "3"
Wait@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/3 /Plug your fido device and press with your thumb/
Show

Hide
Escape
Wait+Screen /Select your authentication method/
Wait+Screen /1\. Password authentication/
Show

Hide
Type "1"
Wait+Prompt /Gimme your password/
Show

Hide
TypeCLIPassword "goodpass"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
