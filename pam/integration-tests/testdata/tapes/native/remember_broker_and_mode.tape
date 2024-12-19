Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
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
Type "r"
Show

Hide
Enter
Wait+Prompt /Choose your authentication method/
Wait+Screen /8\. Authentication code/
Show

Hide
TypeInPrompt "8"
Show

Hide
Enter
Wait+Prompt /Choose action/
Show

Hide
TypeInPrompt "1"
Enter
Wait+Prompt /Enter your one time credential/
Show

Hide
TypeInPrompt "temporary pass0"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show

ClearTerminal

Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Show

Hide
Enter
Wait+Prompt /Choose action/
Show

Hide
TypeInPrompt "1"
Enter
Wait+Prompt /Enter your one time credential/
Show

Hide
TypeInPrompt "temporary pass0"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
