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
Show

Hide
TypeInPrompt "r"
Show

Hide
Enter
Wait+Prompt /Choose your provider/
Show

Hide
TypeInPrompt "invalid-ID"
Show

Hide
Enter
Wait+Screen /Unsupported input/
Wait+Prompt /Choose your provider/
Show

Hide
TypeInPrompt "555"
Show

Hide
Enter
Wait+Screen /Invalid selection/
Wait+Prompt /Choose your provider/
Show

Hide
TypeInPrompt "1"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
