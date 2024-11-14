Hide
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Sleep ${AUTHD_SLEEP_COMMAND}
Show

Hide
Type "2"
Enter
Sleep ${AUTHD_SLEEP_LONG} / 2
Show

Hide
Type "goodpass"
Enter
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Sleep ${AUTHD_SLEEP_LONG}

Hide
Type "echo $USER"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Type '[ -n "${SSH_CONNECTION}" ] && echo "Inside SSH"'
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Hide
Ctrl+D
Sleep ${AUTHD_SLEEP_LONG} * 2
Show

Hide
Type "echo ${SSH_CONNECTION:-Outside SSH}"
Enter
Sleep ${AUTHD_SLEEP_DEFAULT}
Show

Sleep ${AUTHD_SLEEP_DEFAULT}
