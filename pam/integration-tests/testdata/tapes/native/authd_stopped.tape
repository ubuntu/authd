Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Prompt /Choose your provider/
Show

Hide
Sleep ${AUTHD_SLEEP_LONG} * 5
Enter
Wait+Screen /unix:[^\n]+ stopped serving/
Wait
Show
