Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-integration-invalid-new-password"
Wait /Username: user-integration-invalid-new-password$/
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
TypeCLIPassword "noble2404"
Show

Hide
Enter
Wait+Prompt /Confirm password/
TypeCLIPassword "noble2404"
Show

Hide
Enter
Wait+Screen /New password:\n>\n/
Wait /new password does not match criteria/
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

ClearTerminal

# Repeat again, to check that we can use still use another new password

Hide
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "user-integration-invalid-new-password"
Wait /Username: user-integration-invalid-new-password$/
Show

Hide
Enter
Wait+Prompt /Gimme your password/
Show

Hide
TypeCLIPassword "authd2404"
Enter
Wait+Prompt /New password/
Show

Hide
TypeCLIPassword "noble2404"
Show

Hide
Enter
Wait+Prompt /Confirm password/
TypeCLIPassword "noble2404"
Show

Hide
Enter
Wait+Screen /New password:\n>\n/
Wait /new password does not match criteria/
Show

Hide
TypeCLIPassword "goodpass"
Show

Hide
Enter
Wait+Prompt /Confirm password/
TypeCLIPassword "goodpass"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show
