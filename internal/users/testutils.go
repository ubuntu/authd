package users

import (
	"context"
	"fmt"

	"github.com/ubuntu/authd/internal/testsdetection"
)

// IDGeneratorMock is a mock implementation of the IDGenerator interface.
type IDGeneratorMock struct {
	UIDsToGenerate []uint32
	GIDsToGenerate []uint32
}

// GenerateUID generates a UID.
func (g *IDGeneratorMock) GenerateUID(_ context.Context, _ IDOwner) (uint32, error) {
	testsdetection.MustBeTesting()

	if len(g.UIDsToGenerate) == 0 {
		return 0, fmt.Errorf("no more UIDs to generate")
	}
	uid := g.UIDsToGenerate[0]
	g.UIDsToGenerate = g.UIDsToGenerate[1:]
	return uid, nil
}

// GenerateGID generates a GID.
func (g *IDGeneratorMock) GenerateGID(_ context.Context, _ IDOwner) (uint32, error) {
	testsdetection.MustBeTesting()

	if len(g.GIDsToGenerate) == 0 {
		return 0, fmt.Errorf("no more GIDs to generate")
	}
	gid := g.GIDsToGenerate[0]
	g.GIDsToGenerate = g.GIDsToGenerate[1:]
	return gid, nil
}

// ClearPendingIDs clears the pending IDs.
func (g *IDGeneratorMock) ClearPendingIDs() {}
