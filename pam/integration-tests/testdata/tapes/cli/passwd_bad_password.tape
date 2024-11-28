Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-integration-bad-password"
Wait /Username: user-integration-bad-password$/
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
Wait+Prompt /New password/
Show

Hide
Enter
Wait /No password supplied/
Wait+Screen /New password:\n>\n/
Show

Hide
Type "1234"
Enter
Wait /The password is shorter than \d+ characters/
Wait+Screen /New password:\n>\n/
Show

Hide
Type "12345678"
Enter
Wait /The password fails the dictionary check/
Wait+Screen /New password:\n>\n/
Show

Hide
Type "authd2404"
Enter
Wait+Prompt /Confirm password/
Show

Hide
Type "123456789"
Show

Hide
Enter
Wait /Password entries don't match/
Wait+Screen /New password:\n>\n/
Show

Hide
Type "authd2404"
Enter
Wait+Prompt /Confirm password/
Show

Hide
Type "authd2404"
Show

Hide
Enter
Wait+Screen /\nPAM ChangeAuthTok\(\)/
Wait+Screen /\nPAM AcctMgmt\(\)/
Wait
Show
