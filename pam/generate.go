//go:build generate && !pam_module_generation && !pam_debug

//go:generate go generate -C internal/proto

//go:generate ./generate.sh -tags "!pam_debug && withgdmmodel" -build-tags withgdmmodel

package main
