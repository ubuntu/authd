Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
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
Enter
Wait+Nth(1)  /No password supplied/
Wait+Prompt /Enter your new password/
Show

Hide
Type "1234"
Enter
Wait+Nth(1) /The password is shorter than \d+ characters/
Wait+Prompt /Enter your new password/
Show

Hide
Type "12345678"
Enter
Wait+Nth(1) /The password fails the dictionary check/
Wait+Prompt /Enter your new password/
Show

Hide
Type "authd2404"
Enter
Wait+Prompt /Confirm Password/
Show

Hide
Type "123456789"
Enter
Wait+Nth(1) /Password entries don't match/
Wait+Prompt /Enter your new password/
Show

Hide
Enter
Wait+Nth(2)  /No password supplied/
Wait+Prompt /Enter your new password/
Show

Hide
Type "authd2404"
Enter
Wait+Prompt /Confirm Password/
Show

Hide
Type "authd2404"
Enter
Wait
Show
