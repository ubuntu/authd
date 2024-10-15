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
Type "r"
Enter
Wait+Screen /Choose your authentication method:\n>/
Wait
Show

Hide
Type "r"
Enter
Wait+Screen /Choose your provider:\n>/
Wait
Show

Hide
Type "invalid-ID"
Enter
Wait+Screen /Unsupported input/
Wait+Screen /Choose your provider:\n>/
Wait
Show

Hide
Type "555"
Enter
Wait+Screen /Invalid selection/
Wait+Screen /Choose your provider:\n>/
Wait
Show

Hide
Type "1"
Enter
Wait ${AUTHD_TAPE_FINAL_WAIT_PATTERN}
Show
