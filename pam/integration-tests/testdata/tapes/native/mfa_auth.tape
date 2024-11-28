Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Prompt /Choose your provider/
Show

Hide
Type "2"
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "r"
Enter
Wait+Prompt /Choose your authentication method/
Wait+Screen /1\. Password authentication/
Show

Hide
Type "1"
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "goodpass"
Enter
Wait+Prompt@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Plug your fido device and press with your thumb/
Show

Hide
Type "r"
Enter
Wait+Prompt /Choose your authentication method/
Wait+Screen /1\. Password authentication/
Show

Hide
Type "1"
Enter
Wait+Prompt@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Plug your fido device and press with your thumb/
Show

Hide
Type "r"
Enter
Wait+Prompt /Choose your authentication method/
Wait+Screen /2. Use your phone \+33\.\.\./
Show

Hide
Type "2"
Enter
Wait+Prompt /Unlock your phone \+33\.\.\. or accept request on web interface/
Show

Hide
Enter
Wait+Prompt@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}*1.2 /Plug your fido device and press with your thumb/
Show

Hide
Enter
Wait@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}*1.2
Show

Hide
Sleep ${AUTHD_SLEEP_DEFAULT}
Show
