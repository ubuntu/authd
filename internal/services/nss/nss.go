// Package nss implements the nss grpc service protocol to the daemon.
package nss

import (
	"context"

	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/log"
)

type Service struct {
	authd.UnimplementedNssServer
}

func NewService(ctx context.Context) Service {
	log.Debug(ctx, "Building new GRPC NSS service")

	return Service{}
}
