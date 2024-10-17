Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:\n/
Show

Hide
TypeUsername "${AUTHD_TEST_TAPE_USERNAME}"
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
Type "wrongpass"
Enter
Wait+Nth(1) /invalid password 'wrongpass', should be/
Wait+Prompt /Gimme your password/
Show

Hide
Type "wrongpass"
Enter
Wait+Nth(2) /invalid password 'wrongpass', should be/
Wait+Prompt /Gimme your password/
Show

Hide
Type "wrongpass"
Enter
Wait+Nth(3) /invalid password 'wrongpass', should be/
Wait+Prompt /Gimme your password/
Show

Hide
Type "wrongpass"
Enter
Wait+Nth(4) /invalid password 'wrongpass', should be/
Wait+Prompt /Gimme your password/
Show

Hide
Type "wrongpass"
Enter
Wait+Nth(5) /invalid password 'wrongpass', should be/
${AUTHD_TEST_TAPE_COMMAND_PASSWD_FINAL_WAIT}
Show
