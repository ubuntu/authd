Hide
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:$/
Show

Hide
Type "user-integration-invalid-new-password"
Show

Hide
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
Type "noble2404"
Enter
Wait+Screen /Confirm Password:\n>/
Wait
Show

Hide
Type "noble2404"
Show

Hide
Enter
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

# Repeat again, to check that we can use still use another new password

Hide
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:$/
Show

Hide
Type "user-integration-invalid-new-password"
Show

Hide
Enter
Wait+Screen /Gimme your password:\n>/
Wait
Show

Hide
Type "authd2404"
Enter
Wait+Screen /Enter your new password:\n>/
Wait
Show

Hide
Type "noble2404"
Enter
Wait+Screen /Confirm Password:\n>/
Wait
Show

Hide
Type "noble2404"
Enter
Wait+Screen /Enter your new password:\n>/
Wait
Show

Hide
Type "noble2404.1"
Enter
Wait+Screen /Confirm Password:\n>/
Wait
Show

Hide
Type "noble2404.2"
Enter
Wait+Screen /Enter your new password:\n>/
Wait
Show

Hide
Type "goodpass"
Enter
Wait+Screen /Confirm Password:\n>/
Wait
Show

Hide
Type "goodpass"
Enter
Wait
Show
