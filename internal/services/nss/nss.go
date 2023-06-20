// Package nss implements the nss grpc service protocol to the daemon.
package nss

import (
	"context"

	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/log"
)

// Service is the implementation of the NSS module service.
type Service struct {
	authd.UnimplementedNssServer
}

// NewService returns a new NSS GRPC service.
func NewService(ctx context.Context) Service {
	log.Debug(ctx, "Building new GRPC NSS service")

	return Service{}
}
