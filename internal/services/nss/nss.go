// Package nss implements the nss grpc service protocol to the daemon.
package nss

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/ubuntu/authd/internal/brokers"
	"github.com/ubuntu/authd/internal/proto/authd"
	"github.com/ubuntu/authd/internal/services/permissions"
	"github.com/ubuntu/authd/internal/users"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Service is the implementation of the NSS module service.
type Service struct {
	userManager       *users.Manager
	brokerManager     *brokers.Manager
	permissionManager *permissions.Manager

	authd.UnimplementedNSSServer
}

// NewService returns a new NSS GRPC service.
func NewService(ctx context.Context, userManager *users.Manager, brokerManager *brokers.Manager, permissionManager *permissions.Manager) Service {
	log.Debug(ctx, "Building new gRPC NSS service")

	return Service{
		userManager:       userManager,
		brokerManager:     brokerManager,
		permissionManager: permissionManager,
	}
}

// GetPasswdByName returns the passwd entry for the given username.
func (s Service) GetPasswdByName(ctx context.Context, req *authd.GetPasswdByNameRequest) (*authd.PasswdEntry, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "no user name provided")
	}

	u, err := s.userManager.UserByName(req.GetName())
	if err == nil {
		return nssPasswdFromUsersPasswd(u), nil
	}

	if !errors.Is(err, users.NoDataFoundError{}) || !req.GetShouldPreCheck() {
		log.Debugf(context.Background(), "GetPasswdByName: %v", err)
		return nil, noDataFoundErrorToGRPCError(err)
	}

	// If the user is not found in the database, we check if it exists in at least one broker.
	pwent, err := s.userPreCheck(ctx, req.GetName())
	if err != nil {
		log.Debugf(context.Background(), "GetPasswdByName: %v", err)
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return pwent, nil
}

// GetPasswdByUID returns the passwd entry for the given UID.
func (s Service) GetPasswdByUID(ctx context.Context, req *authd.GetByIDRequest) (*authd.PasswdEntry, error) {
	u, err := s.userManager.UserByID(req.GetId())
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
	g, err := s.userManager.GroupByID(req.GetId())
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
	if err := s.permissionManager.IsRequestFromRoot(ctx); err != nil {
		return nil, err
	}

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
	if err := s.permissionManager.IsRequestFromRoot(ctx); err != nil {
		return nil, err
	}

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

// userPreCheck checks if the user exists in at least one broker.
func (s Service) userPreCheck(ctx context.Context, username string) (pwent *authd.PasswdEntry, err error) {
	// Check if the user exists in at least one broker.
	var userinfo string
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
		return nil, fmt.Errorf("user %q is not known by any broker", username)
	}

	var u types.UserEntry
	if err := json.Unmarshal([]byte(userinfo), &u); err != nil {
		return nil, fmt.Errorf("user data from broker invalid: %v", err)
	}

	// Register a temporary user with a unique UID. If the user authenticates successfully, the user will be added to
	// the database with the same UID.
	u.UID, err = s.userManager.RegisterUserPreAuth(u.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to add temporary record for user %q: %v", username, err)
	}

	return nssPasswdFromUsersPasswd(u), nil
}

// nssPasswdFromUsersPasswd returns a PasswdEntry from users.UserEntry.
func nssPasswdFromUsersPasswd(u types.UserEntry) *authd.PasswdEntry {
	return &authd.PasswdEntry{
		Name:    u.Name,
		Passwd:  "x",
		Uid:     u.UID,
		Gid:     u.GID,
		Gecos:   u.Gecos,
		Homedir: u.Dir,
		Shell:   u.Shell,
	}
}

// nssGroupFromUsersGroup returns a GroupEntry from users.GroupEntry.
func nssGroupFromUsersGroup(g types.GroupEntry) *authd.GroupEntry {
	return &authd.GroupEntry{
		Name: g.Name,
		// We set the passwd field here because we use it to store the identifier of a temporary group record
		Passwd:  g.Passwd,
		Gid:     g.GID,
		Members: g.Users,
	}
}

// nssShadowFromUsersShadow returns a ShadowEntry from users.ShadowEntry.
func nssShadowFromUsersShadow(u types.ShadowEntry) *authd.ShadowEntry {
	return &authd.ShadowEntry{
		Name:               u.Name,
		Passwd:             "x",
		LastChange:         convertToNumberOfDays(u.LastPwdChange),
		ChangeMinDays:      convertToNumberOfDays(u.MinPwdAge),
		ChangeMaxDays:      convertToNumberOfDays(u.MaxPwdAge),
		ChangeWarnDays:     convertToNumberOfDays(u.PwdWarnPeriod),
		ChangeInactiveDays: convertToNumberOfDays(u.PwdInactivity),
		ExpireDate:         convertToNumberOfDays(u.ExpirationDate),
	}
}

// noDataFoundErrorToGRPCError converts a data not found to proper GRPC status code.
// This code is picked up by the NSS module to return corresponding NSS status.
func noDataFoundErrorToGRPCError(err error) error {
	if !errors.Is(err, users.NoDataFoundError{}) {
		return err
	}

	return status.Error(codes.NotFound, "")
}

// convertToNumberOfDays returns an int32 from an int. This should be only use for safe conversions where
// we know the numbers can’t be overflow like number of days in shadow.
// We print a warning if the number overflows and replaced it with max int32.
func convertToNumberOfDays(i int) int32 {
	if i > math.MaxInt32 {
		log.Warningf(context.Background(), "Number of days overflows an int32: %d, replaced with max of int32", i)
		return math.MaxInt32
	}
	//nolint:gosec // we did check the conversion beforehand.
	return int32(i)
}
