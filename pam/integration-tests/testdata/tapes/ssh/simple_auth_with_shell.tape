Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Prompt /Choose your provider/
Show

Hide
TypeInPrompt "2"
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "goodpass"
Enter
Wait /\$\n/
Show

Hide
Type "echo $USER"
Enter
Wait /\$\n/
Show

Hide
Type '[ -n "${SSH_CONNECTION}" ] && echo "Inside SSH"'
Enter
Wait /\$\n/
Show

Hide
Ctrl+D
Wait
Show

Hide
Type "echo ${SSH_CONNECTION:-Outside SSH}"
Enter
Wait
Show
