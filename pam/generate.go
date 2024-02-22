//go:build generate && !pam_module_generation && !pam_debug

//go:generate go generate -C internal/proto

//go:generate ./generate.sh -tags "!pam_binary_cli && !pam_debug"

package main
