// Package pam implements the pam grpc service protocol to the daemon.
package pam

import (
	"context"

	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/log"
)

type Service struct {
	authd.UnimplementedPamServer
}

func NewService(ctx context.Context) Service {
	log.Debug(ctx, "Building new GRPC PAM service")

	return Service{}
}
