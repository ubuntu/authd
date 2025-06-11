Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
TypeUsername "user-integration-cli-local-broker"
Show

Hide
Enter
Wait+Screen /Select your provider/
Wait+Screen /1. local/
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show

ClearTerminal

# Repeat the stage again, to ensure the local broker is not auto-selected

Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
TypeUsername "user-integration-cli-local-broker"
Show

Hide
Enter
Wait+Screen /Select your provider/
Wait+Screen /1. local/
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
