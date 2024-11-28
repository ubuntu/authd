Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Screen /(Username:[\n]+){1}/
Show

Hide
Type ""
Enter
Wait+Screen /(Username:[\n]+){2}/
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
