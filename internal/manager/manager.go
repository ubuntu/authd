// Package manager mediates all the business logic of the application.
package manager

import (
	"context"

	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/services/nss"
	"github.com/ubuntu/authd/internal/services/pam"
	"github.com/ubuntu/decorate"
	"google.golang.org/grpc"
)

// Manager mediate the whole business logic of the application.
type Manager struct {
	brokerManager *brokers.Manager
	nssService    nss.Service
	pamService    pam.Service
}

// New returns a new manager after creating all necessary items for our business logic.
func New(ctx context.Context, configuredBrokers []string) (m Manager, err error) {
	defer decorate.OnError(&err /*i18n.G(*/, "can't create authd object") //)

	log.Debug(ctx, "Building authd object")

	brokerManager, err := brokers.NewManager(ctx, configuredBrokers)
	if err != nil {
		return m, err
	}

	nssService := nss.NewService(ctx)
	pamService := pam.NewService(ctx, brokerManager)

	return Manager{
		brokerManager: brokerManager,
		nssService:    nssService,
		pamService:    pamService,
	}, nil
}

// RegisterGRPCServices returns a new grpc Server after registering both NSS and PAM services.
func (a Manager) RegisterGRPCServices(ctx context.Context) *grpc.Server {
	log.Debug(ctx, "Registering GRPC services")

	grpcServer := grpc.NewServer()

	authd.RegisterNSSServer(grpcServer, a.nssService)
	authd.RegisterPAMServer(grpcServer, a.pamService)

	return grpcServer
}
