// TiCS: disabled // This is to pin the tools versions that we use.

//go:build tools

package tools

import (
	_ "github.com/golang/protobuf/protoc-gen-go"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
)
