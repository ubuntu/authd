Output optional_password_reset_accept.txt
Output optional_password_reset_accept.gif # If we don't specify a .gif output, it will create a default out.gif file.

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
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK} force_native_client=true"
Enter
Sleep 300ms
Show

Hide
Type "user-can-reset2"
Enter
Sleep 300ms
Show

Hide
Type "2"
Enter
Sleep 500ms
Show

Hide
Type "goodpass"
Enter
Sleep 2s
Show

Hide
Type "1"
Enter
Sleep 300ms
Show

Hide
Type "authd2404"
Enter
Sleep 2s
Show

Hide
Type "authd2404"
Enter
Sleep 2s
Show

Hide
Enter
Sleep 2s
Show

Sleep 300ms
