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
Wait+Prompt /Choose your provider/
Show

Hide
Type "2"
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "goodpass"
Enter
Wait+Prompt /Enter your new password/
Show

Hide
Type "authd2404"
Enter
Wait+Prompt /Confirm Password/
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
Wait+Prompt /Gimme your password/
Show

Hide
Type "authd2404"
Enter
Wait
Show
