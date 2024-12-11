Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-needs-reset"
Wait /Username: user-needs-reset$/
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
TypeCLIPassword "goodpass"
Show

Hide
Enter
Wait+Prompt /New password/
Show

Hide
Type ""
Show

Hide
Enter
Wait /No password supplied/
Wait+Screen /New password:\n>\n/
Show

Hide
TypeCLIPassword "1234"
Show

Hide
Enter
Wait+Screen /New password:\n>\n/
Wait /The password is shorter than \d+ characters/
Show

Hide
TypeCLIPassword "12345678"
Show

Hide
Enter
Wait /The password fails the dictionary check/
Wait+Screen /New password:\n>\n/
Show

Hide
TypeCLIPassword "authd2404"
Show

Hide
Enter
Wait+Prompt /Confirm password/
Show

Hide
TypeCLIPassword "123456789"
Show

Hide
Enter
Wait /Password entries don't match/
Wait+Screen /New password:\n>\n/
Show

Hide
TypeCLIPassword "authd2404"
Show

Hide
Enter
Wait+Prompt /Confirm password/
Show

Hide
TypeCLIPassword "authd2404"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
