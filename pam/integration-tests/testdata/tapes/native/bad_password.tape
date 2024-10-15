Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Screen /Choose your provider:\n>/
Wait
Show

Hide
Type "2"
Enter
Wait+Screen /Gimme your password:\n>/
Wait
Show

Hide
Type "goodpass"
Enter
Wait+Screen /Enter your new password:\n>/
Wait
Show

Hide
Enter
Wait+Screen /(No password supplied(.|\n)+){1}/
Wait+Screen /Enter your new password:\n>/
Wait
Show

Hide
Type "1234"
Enter
Wait+Screen /(The password is shorter than \d+ characters(.|\n)+){1}/
Wait+Screen /Enter your new password:\n>/
Wait
Show

Hide
Type "12345678"
Enter
Wait+Screen /(The password fails the dictionary check(.|\n)+){1}/
Wait+Screen /Enter your new password:\n>/
Wait
Show

Hide
Type "authd2404"
Enter
Wait+Screen /Confirm Password:\n>/
Wait
Show

Hide
Type "123456789"
Enter
Wait+Screen /(Password entries don't match(.|\n)+){1}/
Wait+Screen /Enter your new password:\n>/
Wait
Show

Hide
Enter
Wait+Screen /(No password supplied(.|\n)+){2}/
Wait+Screen /Enter your new password:\n>/
Wait
Show

Hide
Type "authd2404"
Enter
Wait+Screen /Confirm Password:\n>/
Wait
Show

Hide
Type "authd2404"
Enter
Wait
Show
