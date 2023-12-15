//go:build generate && !pam_debug

//go:generate ./generate.sh -tags "!pam_binary_cli && !pam_debug"

package main
