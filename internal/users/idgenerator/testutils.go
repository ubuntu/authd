package idgenerator

import "fmt"

// IDGeneratorMock is a mock implementation of the IDGenerator interface.
// revive:disable-next-line:exported // We don't want to call this type just "Mock"
type IDGeneratorMock struct {
	UIDsToGenerate []uint32
	GIDsToGenerate []uint32
}

// GenerateUID generates a UID.
func (g *IDGeneratorMock) GenerateUID() (uint32, error) {
	if len(g.UIDsToGenerate) == 0 {
		return 0, fmt.Errorf("no more UIDs to generate")
	}
	uid := g.UIDsToGenerate[0]
	g.UIDsToGenerate = g.UIDsToGenerate[1:]
	return uid, nil
}

// GenerateGID generates a GID.
func (g *IDGeneratorMock) GenerateGID() (uint32, error) {
	if len(g.GIDsToGenerate) == 0 {
		return 0, fmt.Errorf("no more GIDs to generate")
	}
	gid := g.GIDsToGenerate[0]
	g.GIDsToGenerate = g.GIDsToGenerate[1:]
	return gid, nil
}
