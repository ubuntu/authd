Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
TypeUsername "user-mfa"
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
Wait@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Plug your fido device and press with your thumb/
Show

Hide
Escape
Wait+Screen /Select your authentication method/
Wait+Screen /1\. Use your fido device foo/
Show

Hide
Enter
Wait@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Plug your fido device and press with your thumb/
Show

Hide
Wait@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}*1.2 /Unlock your phone \+33\.\.\. or accept request on web interface/
Show

Hide
Escape
Wait+Screen /Select your authentication method/
Wait+Screen /1\. Use your phone \+33\.\.\./
Show

Hide
Enter
Wait@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Unlock your phone \+33\.\.\. or accept request on web interface/
Show

Hide
Wait+Prompt@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}*1.2 /New password/
Show

Hide
TypeCLIPassword "authd2404"
Show

Hide
Enter
Wait+Prompt /Confirm password/
TypeCLIPassword "authd2404"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show
