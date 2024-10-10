//go:build generate

package gdm

//go:generate ../../../tools/generate-proto.sh -I../../.. -I../proto --with-build-tag withgdmmodel gdm.proto
