// Package services mediates all the business logic of the application via a manager.
package services

import (
	"context"

	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/proto/authd"
	"github.com/ubuntu/authd/internal/services/errmessages"
	"github.com/ubuntu/authd/internal/services/nss"
	"github.com/ubuntu/authd/internal/services/pam"
	"github.com/ubuntu/authd/internal/services/permissions"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/decorate"
	"google.golang.org/grpc"
)

// Manager mediate the whole business logic of the application.
type Manager struct {
	userManager   *users.Manager
	brokerManager *brokers.Manager
	pamService    pam.Service
	nssService    nss.Service
}

// NewManager returns a new manager after creating all necessary items for our business logic.
func NewManager(ctx context.Context, cacheDir, brokersConfPath string, configuredBrokers []string, usersConfig users.Config) (m Manager, err error) {
	defer decorate.OnError(&err /*i18n.G(*/, "can't create authd object") //)

	log.Debug(ctx, "Building authd object")

	brokerManager, err := brokers.NewManager(ctx, brokersConfPath, configuredBrokers)
	if err != nil {
		return m, err
	}

	userManager, err := users.NewManager(usersConfig, cacheDir)
	if err != nil {
		return m, err
	}

	permissionManager := permissions.New()

	nssService := nss.NewService(ctx, userManager, brokerManager, &permissionManager)
	pamService := pam.NewService(ctx, userManager, brokerManager, &permissionManager)

	return Manager{
		userManager:   userManager,
		brokerManager: brokerManager,
		nssService:    nssService,
		pamService:    pamService,
	}, nil
}

// RegisterGRPCServices returns a new grpc Server after registering both NSS and PAM services.
func (m Manager) RegisterGRPCServices(ctx context.Context) *grpc.Server {
	log.Debug(ctx, "Registering GRPC services")

	opts := []grpc.ServerOption{permissions.WithUnixPeerCreds(), grpc.ChainUnaryInterceptor(m.globalPermissions, errmessages.RedactErrorInterceptor)}
	grpcServer := grpc.NewServer(opts...)

	authd.RegisterNSSServer(grpcServer, m.nssService)
	authd.RegisterPAMServer(grpcServer, m.pamService)

	return grpcServer
}

// stop stops the underlying cache.
func (m *Manager) stop() error {
	log.Debug(context.TODO(), "Closing grpc manager and cache")

	return m.userManager.Stop()
}
