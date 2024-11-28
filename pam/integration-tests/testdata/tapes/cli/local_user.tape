Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name$/
Show

Hide
Type "root"
Wait /Username: root$/
Show

Hide
Enter
Wait+Screen /\nPAM Authenticate\(\)/
Wait+Screen /\nPAM AcctMgmt\(\)/
Wait
Show
