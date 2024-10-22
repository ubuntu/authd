//go:build tools

package tools

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/msteinert/pam/v2/cmd/pam-moduler"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "github.com/golang/protobuf/protoc-gen-go"
)
