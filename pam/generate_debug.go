//go:build generate && !pam_module_generation && pam_debug

//go:generate go generate -C internal/proto

//go:generate env CFLAGS=-g3 CGO_CFLAGS=-g3 ./generate.sh -tags "!pam_binary_cli && pam_debug" -build-tags pam_gdm_debug -output pam_module_debug.go

package main
