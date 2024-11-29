Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
TypeUsername "user-integration-max-attempts"
Show

Hide
Enter
Wait+Screen /Select your provider/
Wait+Screen /2. ExampleBroker/
Show

Hide
Type "2"
Wait+Prompt /Gimme your password/
Show

Hide
TypeCLIPassword "wrongpass"
Show

Hide
Enter
Wait+CLIPrompt /Gimme your password/ /invalid password '.+', should be[^\n]+/
Show

Hide
TypeCLIPassword "wrongpass"
Show

Hide
Enter
Wait+CLIPrompt /Gimme your password/ /invalid password '.+', should be[^\n]+/
Show

Hide
TypeCLIPassword "wrongpass"
Show

Hide
Enter
Wait+CLIPrompt /Gimme your password/ /invalid password '.+', should be[^\n]+/
Show

Hide
TypeCLIPassword "wrongpass"
Show

Hide
Enter
Wait+CLIPrompt /Gimme your password/ /invalid password '.+', should be[^\n]+/
Show

Hide
TypeCLIPassword "wrongpass"
Show

Hide
Enter
Wait+Screen /invalid password 'wrongpass', should be[^\n]+/
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
