Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
TypeUsername "user-needs-reset-integration-mandatory"
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
TypeCLIPassword "goodpass"
Show

Hide
Enter
Wait /Password reset, 1 step\(s\) missing/
Show

Hide
Wait+Prompt /New password/
Show

Hide
TypeCLIPassword "authd2404"
Show

Hide
Enter
Wait+Prompt /Confirm password/
TypeCLIPassword "authd2404"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
