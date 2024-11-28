Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:$/
Show

Hide
Type "user-mfa"
Show

Hide
Enter
Wait+Screen /Choose your provider:\n>[\n]*$/
Show

Hide
Type "2"
Enter
Wait+Screen /Gimme your password:\n>[\n]*$/
Show

Hide
Type "r"
Enter
Wait+Screen /Choose your authentication method:\n>[\n]*$/
Wait+Screen /1\. Password authentication/
Show

Hide
Type "1"
Enter
Wait+Screen /Gimme your password:\n>[\n]*$/
Show

Hide
Type "goodpass"
Enter
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Plug your fido device and press with your thumb:\n>[\n]*$/
Show

Hide
Type "r"
Enter
Wait+Screen /Choose your authentication method:\n>[\n]*$/
Wait+Screen /1\. Use your fido device foo/
Show

Hide
Type "1"
Enter
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Plug your fido device and press with your thumb:\n>[\n]*$/
Show

Hide
Enter
Show

Hide
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}*1.2 /Unlock your phone \+33\.\.\. or accept request on web interface:\n>[\n]*$/
Wait
Show

Hide
Type "r"
Enter
Wait+Screen /Choose your authentication method:\n>[\n]*$/
Wait+Screen /1\. Use your phone \+33\.\.\./
Show

Hide
Type "1"
Enter
Show

Hide
Wait+Screen /Unlock your phone \+33\.\.\. or accept request on web interface:\n>[\n]*$/
Show

Hide
Enter
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}*1.2 /Enter your new password:\n>[\n]*$/
Show

Hide
Type "authd2404"
Enter
Wait+Screen /Confirm Password:\n>[\n]*$/
Show

Hide
Type "authd2404"
Show

Hide
Enter
Wait
Show
