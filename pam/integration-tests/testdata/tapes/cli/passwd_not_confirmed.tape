Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-integration-wrong-confirmation"
Wait /Username: user-integration-wrong-confirmation$/
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
Type "badpass"
Show

Hide
Enter
Wait /Password entries don't match/
Wait+Screen /New password:\n>\n/
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
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show
