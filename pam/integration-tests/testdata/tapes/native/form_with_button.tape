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
Wait+Screen /8\. Authentication code/
Wait+Prompt /Choose your authentication method/
Show

Hide
TypeInPrompt "8"
Show

Hide
Enter
Wait+Prompt /Choose action/
Show

Hide
TypeInPrompt "2"
Show

Hide
Enter
Wait+Prompt /Choose action/
Show

Hide
TypeInPrompt "1"
Show

Hide
Enter
Wait+Prompt /Enter your one time credential/
Show

Hide
TypeInPrompt "temporary pass00"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
