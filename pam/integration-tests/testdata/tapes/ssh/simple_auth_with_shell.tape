Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Prompt /Choose your provider/
Show

Hide
Type "2"
Enter
Wait+Prompt /Gimme your password/
Show

Hide
Type "goodpass"
Enter
Wait /^\$/
Show

Sleep ${AUTHD_SLEEP_LONG}

Hide
Type "echo $USER"
Enter
Wait /^\$/
Show

Hide
Type '[ -n "${SSH_CONNECTION}" ] && echo "Inside SSH"'
Enter
Wait /^\$/
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
