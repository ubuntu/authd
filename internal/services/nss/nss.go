// Package nss implements the nss grpc service protocol to the daemon.
package nss

import (
	"context"
	"errors"

	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/authd/internal/users"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Service is the implementation of the NSS module service.
type Service struct {
	userManager   *users.Manager
	brokerManager *brokers.Manager
	authd.UnimplementedNSSServer
}

// NewService returns a new NSS GRPC service.
func NewService(ctx context.Context, userManager *users.Manager, brokerManager *brokers.Manager) Service {
	log.Debug(ctx, "Building new GRPC NSS service")

	return Service{
		userManager:   userManager,
		brokerManager: brokerManager,
	}
}

// GetPasswdByName returns the passwd entry for the given username.
func (s Service) GetPasswdByName(ctx context.Context, req *authd.GetByNameRequest) (*authd.PasswdEntry, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "no user name provided")
	}
	u, err := s.userManager.UserByName(req.GetName())
	if err != nil {
		return nil, noDataFoundErrorToGRPCError(err)
	}

	return nssPasswdFromUsersPasswd(u), nil
}

// GetPasswdByUID returns the passwd entry for the given UID.
func (s Service) GetPasswdByUID(ctx context.Context, req *authd.GetByIDRequest) (*authd.PasswdEntry, error) {
	u, err := s.userManager.UserByID(int(req.GetId()))
	if err != nil {
		return nil, noDataFoundErrorToGRPCError(err)
	}

	return nssPasswdFromUsersPasswd(u), nil
}

// GetPasswdEntries returns all passwd entries.
func (s Service) GetPasswdEntries(ctx context.Context, req *authd.Empty) (*authd.PasswdEntries, error) {
	allUsers, err := s.userManager.AllUsers()
	if err != nil {
		return nil, err
	}

	var r authd.PasswdEntries
	for _, u := range allUsers {
		r.Entries = append(r.Entries, nssPasswdFromUsersPasswd(u))
	}

	return &r, nil
}

// GetGroupByName returns the group entry for the given group name.
func (s Service) GetGroupByName(ctx context.Context, req *authd.GetGroupByNameRequest) (*authd.GroupEntry, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "no group name provided")
	}
	g, err := s.userManager.GroupByName(req.GetName())
	if err != nil {
		return nil, noDataFoundErrorToGRPCError(err)
	}

	return nssGroupFromUsersGroup(g), nil
}

// GetGroupByGID returns the group entry for the given GID.
func (s Service) GetGroupByGID(ctx context.Context, req *authd.GetByIDRequest) (*authd.GroupEntry, error) {
	g, err := s.userManager.GroupByID(int(req.GetId()))
	if err != nil {
		return nil, noDataFoundErrorToGRPCError(err)
	}

	return nssGroupFromUsersGroup(g), nil
}

// GetGroupEntries returns all group entries.
func (s Service) GetGroupEntries(ctx context.Context, req *authd.Empty) (*authd.GroupEntries, error) {
	allGroups, err := s.userManager.AllGroups()
	if err != nil {
		return nil, err
	}

	var r authd.GroupEntries
	for _, g := range allGroups {
		r.Entries = append(r.Entries, nssGroupFromUsersGroup(g))
	}

	return &r, nil
}

// GetShadowByName returns the shadow entry for the given username.
func (s Service) GetShadowByName(ctx context.Context, req *authd.GetShadowByNameRequest) (*authd.ShadowEntry, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "no shadow name provided")
	}
	u, err := s.userManager.ShadowByName(req.GetName())
	if err != nil {
		return nil, noDataFoundErrorToGRPCError(err)
	}

	return nssShadowFromUsersShadow(u), nil
}

// GetShadowEntries returns all shadow entries.
func (s Service) GetShadowEntries(ctx context.Context, req *authd.Empty) (*authd.ShadowEntries, error) {
	allUsers, err := s.userManager.AllShadows()
	if err != nil {
		return nil, err
	}

	var r authd.ShadowEntries
	for _, u := range allUsers {
		r.Entries = append(r.Entries, nssShadowFromUsersShadow(u))
	}

	return &r, nil
}

// nssPasswdFromUsersPasswd returns a PasswdEntry from users.UserEntry.
func nssPasswdFromUsersPasswd(u users.UserEntry) *authd.PasswdEntry {
	return &authd.PasswdEntry{
		Name:    u.Name,
		Passwd:  "x",
		Uid:     uint32(u.UID),
		Gid:     uint32(u.GID),
		Gecos:   u.Gecos,
		Homedir: u.Dir,
		Shell:   u.Shell,
	}
}

// nssGroupFromUsersGroup returns a GroupEntry from users.GroupEntry.
func nssGroupFromUsersGroup(g users.GroupEntry) *authd.GroupEntry {
	return &authd.GroupEntry{
		Name:    g.Name,
		Passwd:  "x",
		Gid:     uint32(g.GID),
		Members: g.Users,
	}
}

// nssShadowFromUsersShadow returns a ShadowEntry from users.ShadowEntry.
func nssShadowFromUsersShadow(u users.ShadowEntry) *authd.ShadowEntry {
	return &authd.ShadowEntry{
		Name:               u.Name,
		Passwd:             "x",
		LastChange:         int32(u.LastPwdChange),
		ChangeMinDays:      int32(u.MinPwdAge),
		ChangeMaxDays:      int32(u.MaxPwdAge),
		ChangeWarnDays:     int32(u.PwdWarnPeriod),
		ChangeInactiveDays: int32(u.PwdInactivity),
		ExpireDate:         int32(u.ExpirationDate),
	}
}

// noDataFoundErrorToGRPCError converts a data not found to proper GRPC status code.
// This code is picked up by the NSS module to return corresponding NSS status.
func noDataFoundErrorToGRPCError(err error) error {
	if !errors.Is(err, users.ErrNoDataFound{}) {
		return err
	}

	return status.Error(codes.NotFound, "")
}
