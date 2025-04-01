// Package user provides the gRPC service for user and group management.
package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/proto/authd"
	"github.com/ubuntu/authd/internal/services/permissions"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Service is the implementation of the gRPC user service.
type Service struct {
	userManager       *users.Manager
	brokerManager     *brokers.Manager
	permissionManager *permissions.Manager

	authd.UnimplementedUserServiceServer
}

// NewService returns a new gRPC user service.
func NewService(ctx context.Context, userManager *users.Manager, brokerManager *brokers.Manager, permissionManager *permissions.Manager) Service {
	log.Debug(ctx, "Building new gRPC user service")

	return Service{
		userManager:       userManager,
		brokerManager:     brokerManager,
		permissionManager: permissionManager,
	}
}

// GetUserByName returns the user entry for the given username.
func (s Service) GetUserByName(ctx context.Context, req *authd.GetUserByNameRequest) (*authd.User, error) {
	name := req.GetName()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "no user name provided")
	}

	user, err := s.userManager.UserByName(name)
	if err == nil {
		return userToProtobuf(user), nil
	}

	if !errors.Is(err, users.NoDataFoundError{}) || !req.GetShouldPreCheck() {
		log.Debugf(context.Background(), "GetUserByName: %v", err)
		return nil, grpcError(err)
	}

	// If the user is not found in the database, we check if it exists in at least one broker.
	user, err = s.userPreCheck(ctx, name)
	if err != nil {
		log.Debugf(context.Background(), "GetUserByName: %v", err)
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return userToProtobuf(user), nil
}

// GetUserByID returns the user entry for the given user ID.
func (s Service) GetUserByID(ctx context.Context, req *authd.GetUserByIDRequest) (*authd.User, error) {
	if req.GetId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "no user ID provided")
	}

	u, err := s.userManager.UserByID(req.Id)
	if err != nil {
		return nil, grpcError(err)
	}

	return userToProtobuf(u), nil
}

// ListUsers returns all authd users.
func (s Service) ListUsers(ctx context.Context, req *authd.Empty) (*authd.Users, error) {
	allUsers, err := s.userManager.AllUsers()
	if err != nil {
		return nil, grpcError(err)
	}

	var res authd.Users
	for _, u := range allUsers {
		res.Users = append(res.Users, userToProtobuf(u))
	}

	return &res, nil
}

// GetGroupByName returns the group entry for the given group name.
func (s Service) GetGroupByName(ctx context.Context, req *authd.GetGroupByNameRequest) (*authd.Group, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "no group name provided")
	}

	g, err := s.userManager.GroupByName(req.GetName())
	if err != nil {
		return nil, grpcError(err)
	}

	return groupToProtobuf(g), nil
}

// GetGroupByID returns the group entry for the given group ID.
func (s Service) GetGroupByID(ctx context.Context, req *authd.GetGroupByIDRequest) (*authd.Group, error) {
	if req.GetId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "no group ID provided")
	}

	g, err := s.userManager.GroupByID(req.GetId())
	if err != nil {
		return nil, grpcError(err)
	}

	return groupToProtobuf(g), nil
}

// ListGroups returns all authd groups.
func (s Service) ListGroups(ctx context.Context, req *authd.Empty) (*authd.Groups, error) {
	allGroups, err := s.userManager.AllGroups()
	if err != nil {
		return nil, grpcError(err)
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
		Passwd:  g.Passwd,
	}
}

// userPreCheck checks if the user exists in at least one broker.
func (s Service) userPreCheck(ctx context.Context, username string) (types.UserEntry, error) {
	// authd uses lowercase usernames.
	username = strings.ToLower(username)

	// Check if the user exists in at least one broker.
	var userinfo string
	var err error
	for _, b := range s.brokerManager.AvailableBrokers() {
		// The local broker is not a real broker, so we skip it.
		if b.ID == brokers.LocalBrokerName {
			continue
		}

		userinfo, err = b.UserPreCheck(ctx, username)
		if err == nil && userinfo != "" {
			break
		}
	}

	if err != nil || userinfo == "" {
		return types.UserEntry{}, fmt.Errorf("user %q is not known by any broker", username)
	}

	var u types.UserEntry
	if err := json.Unmarshal([]byte(userinfo), &u); err != nil {
		return types.UserEntry{}, fmt.Errorf("user data from broker invalid: %v", err)
	}

	// Register a temporary user with a unique UID. If the user authenticates successfully, the user will be added to
	// the database with the same UID.
	u.UID, err = s.userManager.RegisterUserPreAuth(u.Name)
	if err != nil {
		return types.UserEntry{}, fmt.Errorf("failed to add temporary record for user %q: %v", username, err)
	}

	return u, nil
}

// grpcError converts a data not found to proper GRPC status code.
// The NSS module uses this status code to determine the NSS status it should return.
func grpcError(err error) error {
	if errors.Is(err, users.NoDataFoundError{}) {
		return status.Error(codes.NotFound, "")
	}

	return err
}
