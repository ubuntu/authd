Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:$/
Show

Hide
Type ""
Enter
Wait /Username:$/
Show

Hide
Type "user-integration-user-selection"
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
Wait
Show
