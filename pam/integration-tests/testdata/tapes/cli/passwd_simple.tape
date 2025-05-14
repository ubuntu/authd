Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
TypeUsername "${AUTHD_TEST_TAPE_USERNAME}"
Show

Hide
Enter
Wait+Screen /Select your provider/
Wait+Screen /2. ExampleBroker/
Show

Hide
Type "2"
Wait+CLIPrompt /Gimme your password/ /Press escape key to go back to select the authentication method/
Show

Hide
TypeCLIPassword "goodpass"
Show

Hide
Enter
Wait+CLIPrompt /New password/ /Press escape key to go back to choose the provider/
Show

Hide
TypeCLIPassword "authd2404"
Show

Hide
Enter
Wait+CLIPrompt /Confirm password/ /Press escape key to go back to choose the provider/
TypeCLIPassword "authd2404"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show

ClearTerminal

Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_LOGIN_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
TypeUsername "${AUTHD_TEST_TAPE_LOGIN_USERNAME}"
Show

Hide
Enter
Wait+CLIPrompt /Gimme your password/ /Press escape key to go back to select the authentication method/
Show

Hide
TypeCLIPassword "authd2404"
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
