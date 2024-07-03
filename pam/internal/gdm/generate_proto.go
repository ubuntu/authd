//go:build generate && proto

//go:generate ../../../tools/generate-proto.sh -I../../.. -I../proto gdm.proto

package gdm
