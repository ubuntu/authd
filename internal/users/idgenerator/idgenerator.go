// Package idgenerator provides an ID generator that generates UIDs and GIDs in a specific range.
package idgenerator

import (
	"errors"
	"sort"

	"github.com/ubuntu/authd/internal/users/localentries"
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
	usedIDs, err := usedIDs()
	if err != nil {
		return 0, err
	}
	return generateID(g.UIDMin, g.UIDMax, usedIDs)
}

// GenerateGID generates a random GID in the configured range.
func (g *IDGenerator) GenerateGID() (uint32, error) {
	usedGIDs, err := usedGIDs()
	if err != nil {
		return 0, err
	}
	return generateID(g.GIDMin, g.GIDMax, usedGIDs)
}

func generateID(minID, maxID uint32, usedIDs []uint32) (uint32, error) {
	if minID > maxID {
		return 0, errors.New("minID must be less than or equal to maxID")
	}

	// Sort usedIDs so we can binary search
	sort.Slice(usedIDs, func(i, j int) bool { return usedIDs[i] < usedIDs[j] })

	// Find the highest used ID, if any
	var highestUsed uint32
	if len(usedIDs) > 0 {
		highestUsed = usedIDs[len(usedIDs)-1]
	} else {
		highestUsed = minID - 1 // No used IDs
	}

	// Try IDs above the highest used
	for id := highestUsed + 1; id <= maxID; id++ {
		// Binary search: is id in usedIDs?
		i := sort.Search(len(usedIDs), func(i int) bool { return usedIDs[i] >= id })
		if i == len(usedIDs) || usedIDs[i] != id {
			return id, nil
		}
	}

	// Fallback: try IDs from minID up to highestUsed
	for id := minID; id <= highestUsed && id <= maxID; id++ {
		i := sort.Search(len(usedIDs), func(i int) bool { return usedIDs[i] >= id })
		if i == len(usedIDs) || usedIDs[i] != id {
			return id, nil
		}
	}

	return 0, errors.New("no available ID in range")
}

func usedIDs() ([]uint32, error) {
	usedUids, err := usedUIDs()
	if err != nil {
		return nil, err
	}

	usedGids, err := usedGIDs()
	if err != nil {
		return nil, err
	}

	return append(usedUids, usedGids...), nil
}

func usedUIDs() ([]uint32, error) {
	var uids []uint32

	userEntries, err := localentries.GetPasswdEntries()
	if err != nil {
		return nil, err
	}

	for _, user := range userEntries {
		uids = append(uids, user.UID)
	}

	return uids, nil
}

func usedGIDs() ([]uint32, error) {
	var gids []uint32

	groupEntries, err := localentries.GetGroupEntries()
	if err != nil {
		return nil, err
	}

	for _, group := range groupEntries {
		gids = append(gids, group.GID)
	}

	return gids, nil
}
