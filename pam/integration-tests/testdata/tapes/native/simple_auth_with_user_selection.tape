Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Nth(1) /Username:/
Show

Hide
Type ""
Enter
Wait+Nth(2) /Username:/
Show

Hide
Type "user-integration-user-selection"
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
Wait
Show
