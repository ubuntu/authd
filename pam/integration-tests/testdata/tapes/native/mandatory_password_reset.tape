Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
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
Wait+Nth(1) /Password reset, 1 step\(s\) missing/
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
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
