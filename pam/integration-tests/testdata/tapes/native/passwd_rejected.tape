Output passwd_rejected.txt
Output passwd_rejected.gif # If we don't specify a .gif output, it will create a default out.gif file.

# Configuration header to standardize the output.
# Does not work with the "Source" command.
Set Width 800
# We need high terminal view, as vhs doesn't scroll:
#  https://github.com/charmbracelet/vhs/issues/404
Set Height 700
# TODO: Ideally, we should use Ubuntu Mono. However, the github runner is still on Jammy, which does not have it.
# We should update this to use Ubuntu Mono once the runner is updated.
Set FontFamily "Monospace"
Set FontSize 13
Set Padding 0
Set Margin 0
Set Shell bash

Hide
Type "./pam_authd passwd socket=${AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK} force_native_client=true"
Enter
Sleep 300ms
Show

Hide
Type "user-integration-invalid-new-password"
Sleep 300ms
Show

Hide
Enter
Sleep 300ms
Show

Hide
Type "2"
Enter
Sleep 300ms
Show

Hide
Type "goodpass"
Enter
Sleep 2s
Show

Hide
Type "noble2404"
Enter
Sleep 300ms
Show

Hide
Type "noble2404"
Sleep 300ms
Show

Hide
Enter
Sleep 2s
Show

Hide
Type "authd2404"
Enter
Sleep 300ms
Show

Hide
Type "authd2404"
Enter
Sleep 300ms
Show

Hide
Enter
Sleep 2s
Show

# Repeat again, to check that we can use still use another new password

Sleep 1s

Hide
Type "./pam_authd passwd socket=${AUTHD_TESTS_CLI_AUTHTOK_TESTS_SOCK} force_native_client=true"
Enter
Sleep 300ms
Show

Hide
Type "user-integration-invalid-new-password"
Sleep 300ms
Show

Hide
Enter
Sleep 300ms
Show

Hide
Type "authd2404"
Enter
Sleep 2s
Show

Hide
Type "noble2404"
Enter
Sleep 300ms
Show

Hide
Type "noble2404"
Sleep 300ms
Show

Hide
Enter
Sleep 2s
Show

Hide
Type "goodpass"
Enter
Sleep 300ms
Show

Hide
Type "goodpass"
Enter
Sleep 300ms
Show

Hide
Enter
Sleep 2s
Show

Sleep 300ms
