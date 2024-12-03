Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-needs-reset-integration-mandatory"
Wait /Username: user-needs-reset-integration-mandatory$/
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
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
