Output passwd_unexistent_user.txt
Output passwd_unexistent_user.gif # If we don't specify a .gif output, it will create a default out.gif file.

# Configuration header to standardize the output.
# Does not work with the "Source" command.
Set Width 800
Set Height 500
# TODO: Ideally, we should use Ubuntu Mono. However, the github runner is still on Jammy, which does not have it.
# We should update this to use Ubuntu Mono once the runner is updated.
Set FontFamily "Monospace"
Set FontSize 13
Set Padding 0
Set Margin 0
Set Shell bash

Hide
Type "./pam_authd passwd socket=${AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK}"
Enter
Sleep 300ms
Show

Hide
Escape
Backspace
Type "user-unexistent"
Sleep 100ms
Show

Hide
Enter
Sleep 100ms
Show

Hide
Type "2"
Sleep 100ms
Show

Sleep 100ms
