Hide
Wait
Type "${AUTHD_TEST_TAPE_COMMAND}"
Enter
Wait+Screen /Choose your provider:\n>[\n]*$/
Show

Hide
Type "2"
Show

Hide
Enter
Wait+Screen /Gimme your password:\n>[\n]*$/
Show

Hide
Type "goodpass"
Show

Hide
Enter
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}/2 /Plug your fido device and press with your thumb:\n>[\n]*$/
Show

# Wait for MFA auth to happen
Hide
Enter
Wait+Screen /Choose action:\n>[\n]*$/
Show

Hide
Type "1"
Enter
Wait+Screen@${AUTHD_SLEEP_EXAMPLE_BROKER_MFA_WAIT}*1.2 /Enter your new password \(3 days until mandatory\):\n>[\n]*$/
Show

Hide
Type "goodpass"
Enter
Wait+Screen /(The password is the same as the old one(.|\n)+){1}/
Wait+Screen /Enter your new password \(3 days until mandatory\):\n>[\n]*$/
Show

Hide
Type "password"
Enter
Wait+Screen /(The password fails the dictionary check(.|\n)+){1}/
Wait+Screen /Enter your new password \(3 days until mandatory\):\n>[\n]*$/
Show

Hide
Type "goodpass"
Enter
Wait+Screen /(The password is the same as the old one(.|\n)+){2}/
Wait+Screen /Enter your new password \(3 days until mandatory\):\n>[\n]*$/
Show

Hide
Type "1234"
Enter
Wait+Screen /(The password is shorter than \d+ characters(.|\n)+){1}/
Wait+Screen /Enter your new password \(3 days until mandatory\):\n>[\n]*$/
Show

Hide
Type "authd2404"
Show

Hide
Enter
Wait+Screen /Confirm Password:\n>[\n]*$/
Show

Hide
Type "authd2404"
Show

Hide
Enter
Wait
Show
