Hide
Type `"$0" -c 'sleep 4 && rm "${AUTHD_TEST_PID_FILE}" &'`
Enter
Type "clear"
Enter
Show

Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
Wait+Screen /unix:[^\n]+ stopped serving/
Wait
Show
