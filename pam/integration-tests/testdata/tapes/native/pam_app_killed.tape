Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND} &"
Enter
Wait

Type "clear"
Enter
Wait

# Find out the PID of the authd PAM binary process, so that we can kill it later.
Type `while true; do sleep 0.2 && child_pid=$(pgrep -f "${AUTHD_TEST_TAPE_AUTHD_PAM_BINARY_NAME} .* socket=${AUTHD_TEST_TAPE_SOCKET}"); [ -n "${child_pid}" ] && break; done`
Enter
Wait

Type "clear"
Enter
Wait

# Get back to the PAM application to foreground, while killing it in the background.
Type `"$0" -c 'sleep 5 && pkill -f "${AUTHD_TEST_TAPE_COMMAND}" &' && clear && fg`
Enter
Wait
Show

ClearTerminal

Hide
# Ensure that the child PID has been killed when destroying the parent application.
Type `if kill -0 "${child_pid}" &>/dev/null; then clear && echo "Parent process still alive"; else clear && echo Parent Process killed; fi`
Enter
Wait /Parent Process killed\n/
Wait ${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
