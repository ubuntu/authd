// Package pam implements the pam grpc service protocol to the daemon.
package pam

import (
	"context"

	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/log"
)

// Service is the implementation of the PAM module service.
type Service struct {
	authd.UnimplementedPamServer
}

// NewService returns a new PAM GRPC service.
func NewService(ctx context.Context) Service {
	log.Debug(ctx, "Building new GRPC PAM service")

	return Service{}
}
