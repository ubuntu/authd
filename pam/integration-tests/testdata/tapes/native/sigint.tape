Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
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
Ctrl+C
Wait+Suffix />[\n]+>/
Show

Hide
Type "echo $TERM"
Enter
Wait
Show
