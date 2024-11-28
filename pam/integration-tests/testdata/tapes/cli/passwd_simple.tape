Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user1"
Wait /Username: user1$/
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
Type "authd2404"
Show

Hide
Enter
Wait+Prompt /Confirm password/
Type "authd2404"
Show

Hide
Enter
Wait+Screen /\nPAM ChangeAuthTok\(\)/
Wait+Screen /\nPAM AcctMgmt\(\)/
Wait
Show

Hide
Type "${AUTHD_TEST_TAPE_LOGIN_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user1"
Wait+Screen /Username: user1\n/
Show

Hide
Enter
Wait+Screen /Gimme your password/
Wait
Show

Hide
Type "authd2404"
Enter
Wait+Screen /\nPAM Authenticate\(\)/
Wait+Screen /\nPAM AcctMgmt\(\)/
Wait
Show
