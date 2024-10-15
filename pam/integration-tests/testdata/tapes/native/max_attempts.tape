Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Screen /Choose your provider:\n>/
Wait
Show

Hide
Type "2"
Enter
Wait+Screen /Gimme your password:\n>/
Wait
Show

Hide
Type "wrongpass"
Enter
Wait+Screen /(invalid password 'wrongpass', should be(.|\n)+){1}/
Wait+Screen /Gimme your password:\n>/
Wait
Show

Hide
Type "wrongpass"
Enter
Wait+Screen /(invalid password 'wrongpass', should be(.|\n)+){2}/
Wait+Screen /Gimme your password:\n>/
Wait
Show

Hide
Type "wrongpass"
Enter
Wait+Screen /(invalid password 'wrongpass', should be(.|\n)+){3}/
Wait+Screen /Gimme your password:\n>/
Wait
Show

Hide
Type "wrongpass"
Enter
Wait+Screen /(invalid password 'wrongpass', should be(.|\n)+){4}/
Wait+Screen /Gimme your password:\n>/
Wait
Show

Hide
Type "wrongpass"
Enter
Wait+Screen /(invalid password 'wrongpass', should be(.|\n)+){5}/
Wait
Show
