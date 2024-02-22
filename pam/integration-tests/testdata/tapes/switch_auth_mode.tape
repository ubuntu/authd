Output switch_auth_mode.txt
Output switch_auth_mode.gif # If we don't specify a .gif output, it will create a default out.gif file.

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
Type "./pam_authd login socket=/tmp/pam-cli-authenticate-tests.sock"
Enter
Sleep 100ms
Show

Hide
Escape
Backspace
Type "user-integration-switch-mode"
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

Hide
Escape
Sleep 100ms
Show

Hide
Type "2"
Sleep 100ms
Show

Hide
Escape
Sleep 100ms
Show

Hide
Type "3"
Sleep 100ms
Show

Hide
Escape
Sleep 100ms
Show

Hide
Type "1"
Sleep 100ms
Show

Hide
Type "goodpass"
Enter
Sleep 2s
Show

Sleep 100ms
