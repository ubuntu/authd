// Package cli implements the gRPC service to be used by the CLI.
package cli

import (
	"context"

	"github.com/ubuntu/authd/internal/proto/authd"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
)

// Service is the gRPC service for the CLI.
type Service struct {
	userManager *users.Manager

	authd.UnimplementedCLIServer
}

// NewService returns a new CLI GRPC service.
func NewService(ctx context.Context, userManager *users.Manager) Service {
	log.Debug(ctx, "Building new gRPC CLI service")

	return Service{
		userManager: userManager,
	}
}

// ListUsers returns all authd users.
func (s Service) ListUsers(ctx context.Context, req *authd.Empty) (*authd.Users, error) {
	allUsers, err := s.userManager.AllUsers()
	if err != nil {
		return nil, err
	}

	var r authd.Users
	for _, u := range allUsers {
		r.Users = append(r.Users, cliUserFromUserEntry(u))
	}

	return &r, nil
}

// ListGroups returns all authd groups.
func (s Service) ListGroups(ctx context.Context, req *authd.Empty) (*authd.Groups, error) {
	allGroups, err := s.userManager.AllGroups()
	if err != nil {
		return nil, err
	}

	var r authd.Groups
	for _, g := range allGroups {
		r.Groups = append(r.Groups, cliGroupFromUsersGroup(g))
	}

	return &r, nil
}

// cliUserFromUserEntry converts a types.UserEntry to authd.User.
func cliUserFromUserEntry(u types.UserEntry) *authd.User {
	return &authd.User{
		Name:    u.Name,
		Uid:     u.UID,
		Gid:     u.GID,
		Gecos:   u.Gecos,
		Homedir: u.Dir,
		Shell:   u.Shell,
	}
}

// cliGroupFromUsersGroup converts a types.GroupEntry to authd.Group.
func cliGroupFromUsersGroup(g types.GroupEntry) *authd.Group {
	return &authd.Group{
		Name:    g.Name,
		Gid:     g.GID,
		Members: g.Users,
	}
}
