Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-integration-max-attempts"
Wait /Username: user-integration-max-attempts$/
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
Type "wrongpass"
Show

Hide
Enter
Wait /invalid password/
Wait+Screen /Gimme your password\n>\n/
Show

Hide
Type "wrongpass"
Show

Hide
Enter
Wait /invalid password/
Wait+Screen /Gimme your password\n>\n/
Show

Hide
Type "wrongpass"
Show

Hide
Enter
Wait /invalid password/
Wait+Screen /Gimme your password\n>\n/
Show

Hide
Type "wrongpass"
Show

Hide
Enter
Wait /invalid password/
Wait+Screen /Gimme your password\n>\n/
Show

Hide
Type "wrongpass"
Show

Hide
Enter
Wait+Screen /invalid password/
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show
