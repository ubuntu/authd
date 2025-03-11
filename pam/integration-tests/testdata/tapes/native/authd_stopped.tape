Hide
Type `"$0" -c 'sleep 4 && rm "${AUTHD_TEST_PID_FILE}" &'`
Enter
Type "clear"
Enter
Show

Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Prompt /Choose your provider/
Show

Hide
Sleep ${AUTHD_SLEEP_LONG} * 5
Enter
Wait+Screen /unix:[^\n]+ stopped serving/
Wait
Show
