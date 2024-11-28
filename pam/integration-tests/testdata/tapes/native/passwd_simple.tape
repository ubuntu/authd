Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:$/
Show

Hide
Type "user1"
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

Hide
Type "${AUTHD_TEST_TAPE_LOGIN_COMMAND}"
Enter
Wait /Username:$/
Show

Hide
Type "user1"
Show

Hide
Enter
Wait+Screen /Gimme your password:\n>[\n]*$/
Show

Hide
Type "authd2404"
Enter
Wait
Show
