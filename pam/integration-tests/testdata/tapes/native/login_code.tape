Output login_code.txt
Output login_code.gif # If we don't specify a .gif output, it will create a default out.gif file.

# Configuration header to standardize the output.
# Does not work with the "Source" command.
Set Width 800
# We need high terminal view, as vhs doesn't scroll:
#  https://github.com/charmbracelet/vhs/issues/404
Set Height 1050
# TODO: Ideally, we should use Ubuntu Mono. However, the github runner is still on Jammy, which does not have it.
# We should update this to use Ubuntu Mono once the runner is updated.
Set FontFamily "Monospace"
Set FontSize 13
Set Padding 0
Set Margin 0
Set Shell bash

Hide
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK} force_native_client=true disable_qrcode_rendering=true"
Enter
Sleep 300ms
Show

Hide
Type "user-integration-login-code"
Sleep 300ms
Show

Hide
Enter
Sleep 300ms
Show

Hide
Type "2"
Enter
Sleep 500ms
Show

Hide
Type "r"
Enter
Sleep 300ms
Show

Hide
Type "2"
Enter
Sleep 300ms
Show

Hide
Type "2"
Enter
Sleep 500ms
Show

Hide
Type "2"
Enter
Sleep 500ms
Show

Hide
Type "2"
Enter
Sleep 500ms
Show

Hide
Type "2"
Enter
Sleep 500ms
Show

Hide
Type "1"
Enter
Sleep 300ms
Show

Hide
Sleep 3s
Show

Sleep 300ms
