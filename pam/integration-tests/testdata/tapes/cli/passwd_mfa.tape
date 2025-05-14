Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
TypeUsername "${AUTHD_TEST_TAPE_USERNAME}"
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
Escape
Wait+Screen /Select your authentication method/
Wait+Screen /1\. Password authentication/
Show

Hide
Type "1"
Wait+CLIPrompt /Gimme your password/ /Press escape key to go back to select the authentication method/
Show

Hide
TypeCLIPassword "goodpass"
Show

Hide
Enter
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Plug your fido device and press with your thumb/
Wait /Press escape key to go back to select the authentication method/
Show

Hide
Escape
Wait+Screen /Select your authentication method/
Wait+Screen /1\. Use your fido device foo/
Show

Hide
Enter
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Plug your fido device and press with your thumb/
Wait /Press escape key to go back to select the authentication method/
Show

Hide
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}*1.2 /Unlock your phone \+33\.\.\. or accept request on web interface/
Wait /Press escape key to go back to select the authentication method/
Show

Hide
Escape
Wait+Screen /Select your authentication method/
Wait+Screen /1\. Use your phone \+33\.\.\./
Show

Hide
Enter
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Unlock your phone \+33\.\.\. or accept request on web interface/
Wait /Press escape key to go back to select the authentication method/
Show

Hide
Wait+CLIPrompt@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}*1.2 /New password/ /Press escape key to go back to choose the provider/
Show

Hide
TypeCLIPassword "authd2404"
Show

Hide
Enter
Wait+CLIPrompt /Confirm password/ /Press escape key to go back to choose the provider/
TypeCLIPassword "authd2404"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show
