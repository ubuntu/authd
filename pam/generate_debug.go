//go:build generate && !pam_module_generation && pam_debug

//go:generate go generate -C internal/proto

//go:generate env CFLAGS=-g3 CGO_CFLAGS=-g3 ./generate.sh -tags "pam_debug && withgdmmodel" -build-tags "pam_gdm_debug,withgdmmodel" -build-flags "\"-gcflags=all=-N -l\"" -output pam_module_debug.go

package main
