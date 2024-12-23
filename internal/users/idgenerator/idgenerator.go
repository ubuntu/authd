// Package idgenerator provides an ID generator that generates UIDs and GIDs in a specific range.
package idgenerator

import (
	"crypto/rand"
	"math/big"
)

// IDGenerator is an ID generator that generates UIDs and GIDs in a specific range.
type IDGenerator struct {
	UIDMin uint32
	UIDMax uint32
	GIDMin uint32
	GIDMax uint32
}

// GenerateUID generates a random UID in the configured range.
func (g *IDGenerator) GenerateUID() (uint32, error) {
	return generateID(g.UIDMin, g.UIDMax)
}

// GenerateGID generates a random GID in the configured range.
func (g *IDGenerator) GenerateGID() (uint32, error) {
	return generateID(g.GIDMin, g.GIDMax)
}

func generateID(minID, maxID uint32) (uint32, error) {
	diff := int64(maxID - minID)
	// Generate a cryptographically secure random number between 0 and diff
	nBig, err := rand.Int(rand.Reader, big.NewInt(diff+1))
	if err != nil {
		return 0, err
	}

	// Add minID to get a number in the desired range
	//nolint:gosec // This conversion is safe because we only generate UIDs which are positive and smaller than uint32.
	return uint32(nBig.Int64()) + minID, nil
}
