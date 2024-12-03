Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait /Username:$/
Show

Hide
Type "user-integration-max-attempts"
Enter
Wait+Prompt /Choose your provider/
Show

Hide
Type "2"
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
