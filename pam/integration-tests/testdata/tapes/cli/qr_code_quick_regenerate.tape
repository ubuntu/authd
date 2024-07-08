Output qr_code_quick_regenerate.txt
Output qr_code_quick_regenerate.gif # If we don't specify a .gif output, it will create a default out.gif file.

# Configuration header to standardize the output.
# Does not work with the "Source" command.
Set Width 800
Set Height 650
# TODO: Ideally, we should use Ubuntu Mono. However, the github runner is still on Jammy, which does not have it.
# We should update this to use Ubuntu Mono once the runner is updated.
Set FontFamily "Monospace"
Set FontSize 13
Set Padding 0
Set Margin 0
Set Shell bash

Hide
Type "./pam_authd login socket=${AUTHD_TESTS_CLI_AUTHENTICATE_TESTS_SOCK}"
Enter
Sleep 300ms
Show

Hide
Escape
Backspace
Type "user-integration-qrcode-regenerate"
Sleep 300ms
Show

Hide
Enter
Sleep 300ms
Show

Hide
Type "2"
Sleep 300ms
Show

Hide
Escape
Sleep 300ms
Show

Hide
Type "6"
Sleep 300ms
Show

Hide
Tab
Sleep 300ms
Show

Hide
Enter@1ms 50
Sleep 10s
Show

Sleep 300ms
