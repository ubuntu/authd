Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-mfa-with-reset"
Wait /Username: user-mfa-with-reset$/
Show

Hide
Enter
Wait+Screen /Select your provider/
Wait+Screen /2. ExampleBroker/
Show

Hide
Type "2"
Wait+Screen /Gimme your password/
Wait
Show

Hide
Type "goodpass"
Show

Hide
Enter
Wait@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Plug your fido device and press with your thumb/
Show

# Wait for MFA auth to happen
Hide
Wait+Screen @${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} * 1.2 /Enter your new password \(3 days until mandatory\)/
Wait+Screen @${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} * 1.2 /New password:\n>\n/
Show

Hide
Type "goodpass"
Show

Hide
Enter
Wait /The password is the same as the old one/
Wait+Screen /New password:\n>\n/
Show

Hide
Type "password"
Show

Hide
Enter
Wait /The password fails the dictionary check/
Wait+Screen /New password:\n>\n/
Show

Hide
Type "goodpass"
Show

Hide
Enter
Wait /The password is the same as the old one/
Wait+Screen /New password:\n>\n/
Show

Hide
Type "1234"
Show

Hide
Enter
Wait /The password is shorter than \d+ characters/
Wait+Screen /New password:\n>\n/
Show

Hide
Type "authd2404"
Show

Hide
Enter
Wait+Screen /Confirm password:\n>\n/
Show

Hide
Type "authd2404"
Show

Hide
Enter
Wait+Screen /\nPAM Authenticate\(\)/
Wait+Screen /\nPAM AcctMgmt\(\)/
Wait
Show
