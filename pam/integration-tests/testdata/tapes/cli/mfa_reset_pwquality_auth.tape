Hide
TypeInPrompt+Shell "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username: user name\n/
Show

Hide
TypeUsername "user-mfa-with-reset"
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
Wait@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Plug your fido device and press with your thumb/
Show

# Wait for MFA auth to happen
Hide
Wait+Screen @${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} * 1.2 /Enter your new password \(3 days until mandatory\)\n/
Wait+CLIPrompt@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT} * 1.2 /New password/ /\[ Skip \]/
Show

Hide
TypeCLIPassword "goodpass"
Show

Hide
Enter
Wait+CLIPrompt@${AUTHD_WAIT_DEFAULT}*2 /New password/ /\[ Skip \]/ /The password is the same as the old one/
Show

Hide
TypeCLIPassword "password"
Show

Hide
Enter
Wait+CLIPrompt@${AUTHD_WAIT_DEFAULT}*2 /New password/ /\[ Skip \]\nThe password fails the dictionary check[^\n]+/
Show

Hide
TypeCLIPassword "goodpass"
Show

Hide
Enter
Wait+CLIPrompt@${AUTHD_WAIT_DEFAULT}*2 /New password/ /\[ Skip \]\nThe password is the same as the old one/
Show

Hide
TypeCLIPassword "1234"
Show

Hide
Enter
Wait+CLIPrompt@${AUTHD_WAIT_DEFAULT}*2 /New password/ /\[ Skip \]\nThe password is shorter than \d+ characters/
Show

Hide
TypeCLIPassword "authd2404"
Show

Hide
Enter
Wait+CLIPrompt@${AUTHD_WAIT_DEFAULT}*2 /Confirm password/ /\[ Skip \]/
Show

Hide
TypeCLIPassword "authd2404"
Show

Hide
Enter
${AUTHD_TEST_TAPE_COMMAND_AUTH_FINAL_WAIT}
Show
