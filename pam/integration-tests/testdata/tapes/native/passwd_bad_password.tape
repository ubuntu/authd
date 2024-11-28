Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:$/
Show

Hide
Type "user-integration-bad-password"
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
Type "goodpass"
Enter
Wait+Screen /Enter your new password:\n>[\n]*$/
Show

Hide
Enter
Wait+Screen /Enter your new password:\n>[\n]*$/
Show

Hide
Type "1234"
Enter
Wait+Screen /Enter your new password:\n>[\n]*$/
Show

Hide
Type "12345678"
Enter
Wait+Screen /Enter your new password:\n>[\n]*$/
Show

Hide
Type "authd2404"
Enter
Wait+Screen /Confirm Password:\n>[\n]*$/
Show

Hide
Type "123456789"
Enter
Wait+Screen /Enter your new password:\n>[\n]*$/
Show

Hide
Enter
Wait+Screen /Enter your new password:\n>[\n]*$/
Show

Hide
Type "authd2404"
Enter
Wait+Screen /Confirm Password:\n>[\n]*$/
Show

Hide
Type "authd2404"
Enter
Wait
Show
