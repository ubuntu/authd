//go:build generate && !pam_debug

//go:generate go generate -C internal/proto

//go:generate ./generate.sh -tags "!pam_binary_cli && !pam_debug" -no-generator

package main
