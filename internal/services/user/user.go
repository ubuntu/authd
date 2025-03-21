// Package user provides the gRPC service for user and group management.
package user

import (
	"context"

	"github.com/ubuntu/authd/internal/proto/authd"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
)

// Service is the implementation of the gRPC user service.
type Service struct {
	userManager *users.Manager

	authd.UnimplementedUserServiceServer
}

// NewService returns a new gRPC user service.
func NewService(ctx context.Context, userManager *users.Manager) Service {
	log.Debug(ctx, "Building new gRPC user service")

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

	var res authd.Users
	for _, u := range allUsers {
		res.Users = append(res.Users, userToProtobuf(u))
	}

	return &res, nil
}

// ListGroups returns all authd groups.
func (s Service) ListGroups(ctx context.Context, req *authd.Empty) (*authd.Groups, error) {
	allGroups, err := s.userManager.AllGroups()
	if err != nil {
		return nil, err
	}

	var res authd.Groups
	for _, g := range allGroups {
		res.Groups = append(res.Groups, groupToProtobuf(g))
	}

	return &res, nil
}

// userToProtobuf converts a types.UserEntry to authd.User.
func userToProtobuf(u types.UserEntry) *authd.User {
	return &authd.User{
		Name:    u.Name,
		Uid:     u.UID,
		Gid:     u.GID,
		Gecos:   u.Gecos,
		Homedir: u.Dir,
		Shell:   u.Shell,
	}
}

// groupToProtobuf converts a types.GroupEntry to authd.Group.
func groupToProtobuf(g types.GroupEntry) *authd.Group {
	return &authd.Group{
		Name:    g.Name,
		Gid:     g.GID,
		Members: g.Users,
	}
}
