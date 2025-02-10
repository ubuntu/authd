Hide
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:\n/
Show

Hide
TypeUsername "${AUTHD_TEST_TAPE_USERNAME}"
Enter
Wait+Prompt /Choose your provider/
Show

Hide
TypeInPrompt "2"
Show

Hide
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "goodpass"
Enter
Wait+Prompt /Enter your new password/
Show

Hide
Type "noble2404"
Enter
Wait+Prompt /Confirm Password/
Show

Hide
Type "noble2404"
Show

Hide
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
Enter
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show

ClearTerminal

# Repeat again, to check that we can use still use another new password

Hide
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:\n/
Show

Hide
TypeUsername "${AUTHD_TEST_TAPE_USERNAME}"
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "authd2404"
Enter
Wait+Prompt /Enter your new password/
Show

Hide
Type "noble2404"
Enter
Wait+Prompt /Confirm Password/
Show

Hide
Type "noble2404"
Enter
Wait+Prompt /Enter your new password/
Show

Hide
Type "noble2404.1"
Enter
Wait+Prompt /Confirm Password/
Show

Hide
Type "noble2404.2"
Enter
Wait+Prompt /Enter your new password/
Show

Hide
Type "goodpass"
Enter
Wait+Prompt /Confirm Password/
Show

Hide
Type "goodpass"
Enter
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show
