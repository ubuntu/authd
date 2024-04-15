package services

import (
	"context"
	"strings"

	"google.golang.org/grpc"
)

func (m Manager) globalPermissions(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if strings.HasPrefix(info.FullMethod, "/authd.PAM/") {
		if err := m.pamService.CheckGlobalAccess(ctx, info.FullMethod); err != nil {
			return nil, err
		}
	} else if strings.HasPrefix(info.FullMethod, "/authd.NSS/") {
		if err := m.nssService.CheckGlobalAccess(ctx, info.FullMethod); err != nil {
			return nil, err
		}
	}

	return handler(ctx, req)
}
