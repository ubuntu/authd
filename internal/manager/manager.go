// Package manager mediates all the business logic of the application.
package manager

import (
	"context"

	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/services/nss"
	"github.com/ubuntu/authd/internal/services/pam"
	"github.com/ubuntu/decorate"
	"google.golang.org/grpc"
)

// Manager mediate the whole business logic of the application
type Manager struct {
	nssService nss.Service
	pamService pam.Service
}

// New returns a new manager after creating all necessary items for our business logic.
func New(ctx context.Context) (m Manager, err error) {
	defer decorate.OnError(&err /*i18n.G(*/, "can't create authd object") //)

	log.Debug(ctx, "Building authd object")

	nssService := nss.NewService(ctx)
	pamService := pam.NewService(ctx)

	return Manager{
		nssService: nssService,
		pamService: pamService,
	}, nil
}

func (a Manager) RegisterGRPCServices(ctx context.Context) *grpc.Server {
	log.Debug(ctx, "Registering GRPC services")

	grpcServer := grpc.NewServer()

	authd.RegisterNssServer(grpcServer, a.nssService)
	authd.RegisterPamServer(grpcServer, a.pamService)

	return grpcServer
}
