// Package nss implements the nss grpc service protocol to the daemon.
package nss

import (
	"context"
	"fmt"

	"github.com/ubuntu/authd"
	"github.com/ubuntu/authd/internal/log"
)

// Service is the implementation of the NSS module service.
type Service struct {
	authd.UnimplementedNSSServer
}

// NewService returns a new NSS GRPC service.
func NewService(ctx context.Context) Service {
	log.Debug(ctx, "Building new GRPC NSS service")

	return Service{}
}

// GetPasswdByName returns the passwd entry for the given username.
func (s Service) GetPasswdByName(ctx context.Context, req *authd.GetByNameRequest) (*authd.PasswdEntry, error) {
	if req.GetName() != "static-user" {
		return nil, fmt.Errorf("user %q not found", req.GetName())
	}
	return &authd.PasswdEntry{
		Name:    "static-user",
		Passwd:  "x",
		Uid:     1111,
		Gid:     1111,
		Gecos:   "Static User",
		Homedir: "/home/static-user/",
		Shell:   "/bin/bash",
	}, nil
}

// GetPasswdByUID returns the passwd entry for the given UID.
func (s Service) GetPasswdByUID(ctx context.Context, req *authd.GetByIDRequest) (*authd.PasswdEntry, error) {
	if req.GetId() != 1111 {
		return nil, fmt.Errorf("user with ID %d not found", req.GetId())
	}

	return &authd.PasswdEntry{
		Name:    "static-user",
		Passwd:  "x",
		Uid:     1111,
		Gid:     1111,
		Gecos:   "Static User",
		Homedir: "/home/static-user/",
		Shell:   "/bin/bash",
	}, nil
}

// GetPasswdEntries returns all passwd entries.
func (s Service) GetPasswdEntries(ctx context.Context, req *authd.Empty) (*authd.PasswdEntries, error) {
	return &authd.PasswdEntries{
		Entries: []*authd.PasswdEntry{
			{
				Name:    "static-user",
				Passwd:  "x",
				Uid:     1111,
				Gid:     1111,
				Gecos:   "Static User",
				Homedir: "/home/static-user/",
				Shell:   "/bin/bash",
			},
			{
				Name:    "static-user-2",
				Passwd:  "x",
				Uid:     2222,
				Gid:     3333,
				Gecos:   "Static User 2",
				Homedir: "/home/static-user-2/",
				Shell:   "/bin/bash",
			},
		},
	}, nil
}

// GetGroupByName returns the group entry for the given group name.
func (s Service) GetGroupByName(ctx context.Context, req *authd.GetByNameRequest) (*authd.GroupEntry, error) {
	if req.GetName() != "static-user" {
		return nil, fmt.Errorf("group %q not found", req.GetName())
	}

	return &authd.GroupEntry{
		Name:   "static-user",
		Passwd: "x",
		Gid:    1111,
		Members: []string{
			"static-user",
		},
	}, nil
}

// GetGroupByGID returns the group entry for the given GID.
func (s Service) GetGroupByGID(ctx context.Context, req *authd.GetByIDRequest) (*authd.GroupEntry, error) {
	if req.GetId() != 1111 {
		return nil, fmt.Errorf("group with ID %d not found", req.GetId())
	}

	return &authd.GroupEntry{
		Name:   "static-user",
		Passwd: "x",
		Gid:    1111,
		Members: []string{
			"static-user",
		},
	}, nil
}

// GetGroupEntries returns all group entries.
func (s Service) GetGroupEntries(ctx context.Context, req *authd.Empty) (*authd.GroupEntries, error) {
	return &authd.GroupEntries{
		Entries: []*authd.GroupEntry{
			{
				Name:   "static-group",
				Passwd: "x",
				Gid:    1111,
				Members: []string{
					"static-user",
				},
			},
			{
				Name:   "static-group-2",
				Passwd: "x",
				Gid:    3333,
				Members: []string{
					"static-user",
					"static-user-2",
				},
			},
		},
	}, nil
}

// GetShadowByName returns the shadow entry for the given username.
func (s Service) GetShadowByName(ctx context.Context, req *authd.GetByNameRequest) (*authd.ShadowEntry, error) {
	if req.GetName() != "static-user" {
		return nil, fmt.Errorf("user %q not found", req.GetName())
	}

	return &authd.ShadowEntry{
		Name:               "static-user",
		Passwd:             "",
		LastChange:         -1,
		ChangeMinDays:      -1,
		ChangeMaxDays:      -1,
		ChangeWarnDays:     -1,
		ChangeInactiveDays: -1,
		ExpireDate:         -1,
	}, nil
}

// GetShadowEntries returns all shadow entries.
func (s Service) GetShadowEntries(ctx context.Context, req *authd.Empty) (*authd.ShadowEntries, error) {
	return &authd.ShadowEntries{
		Entries: []*authd.ShadowEntry{
			{
				Name:               "static-user",
				Passwd:             "",
				LastChange:         -1,
				ChangeMinDays:      -1,
				ChangeMaxDays:      -1,
				ChangeWarnDays:     -1,
				ChangeInactiveDays: -1,
				ExpireDate:         -1,
			},
			{
				Name:               "static-user-2",
				Passwd:             "",
				LastChange:         -1,
				ChangeMinDays:      -1,
				ChangeMaxDays:      -1,
				ChangeWarnDays:     -1,
				ChangeInactiveDays: -1,
				ExpireDate:         -1,
			},
		},
	}, nil
}
