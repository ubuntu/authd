package users

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"

	"github.com/ubuntu/authd/internal/users/localentries"
	"github.com/ubuntu/authd/log"
)

// IDGeneratorIface is the interface that must be implemented by the ID generator.
type IDGeneratorIface interface {
	GenerateUID(ctx context.Context, owner IDOwner) (uint32, error)
	GenerateGID(ctx context.Context, owner IDOwner) (uint32, error)
	ClearPendingIDs()
}

// IDOwner is the interface that must be implemented by the IDs owner to provide
// the currently used UIDs and GIDs.
type IDOwner interface {
	UsedUIDs() ([]uint32, error)
	UsedGIDs() ([]uint32, error)
}

// IDGenerator is an ID generator that generates UIDs and GIDs in a specific range.
type IDGenerator struct {
	UIDMin uint32
	UIDMax uint32
	GIDMin uint32
	GIDMax uint32

	// IDs generated but not saved to the database yet.
	// This is used to avoid generating the same ID multiple times.
	// We don't differentiate between UIDs and GIDs here, because:
	// * When picking a UID, we avoid IDs which we already used as GIDs,
	//   because the UID is also used as the GID of the user private group.
	// * When picking a GID, we avoid IDs which we already used as UIDs,
	//   because those are also GIDs of the user private groups.
	pendingIDs   []uint32
	pendingIDsMu sync.Mutex
}

// Avoid to loop forever if we can't find an UID for the user, it's just better
// to fail after a limit is reached than hang or crash.
const maxIDGenerateIterations = 1000000

// GenerateUID generates a random UID in the configured range.
func (g *IDGenerator) GenerateUID(ctx context.Context, owner IDOwner) (uint32, error) {
	return g.generateID(ctx, owner, generateID{
		idType:        "UID",
		minID:         g.UIDMin,
		maxID:         g.UIDMax,
		getUsedIDs:    g.getUsedIDs,
		isAvailableID: g.isUIDAvailable,
	})
}

// GenerateGID generates a random GID in the configured range.
func (g *IDGenerator) GenerateGID(ctx context.Context, owner IDOwner) (uint32, error) {
	return g.generateID(ctx, owner, generateID{
		idType:        "GID",
		minID:         g.GIDMin,
		maxID:         g.GIDMax,
		getUsedIDs:    g.getUsedGIDs,
		isAvailableID: g.isGIDAvailable,
	})
}

// This is an utility struct to allow code sharing simplifying the arguments passing.
type generateID struct {
	idType        string
	minID, maxID  uint32
	isAvailableID func(context.Context, uint32) (bool, error)
	getUsedIDs    func(context.Context, IDOwner) ([]uint32, error)
}

func (g *IDGenerator) generateID(ctx context.Context, owner IDOwner, args generateID) (id uint32, err error) {
	g.pendingIDsMu.Lock()
	defer g.pendingIDsMu.Unlock()

	usedIDs, err := args.getUsedIDs(ctx, owner)
	if err != nil {
		return 0, err
	}

	// Add pending IDs to the used IDs to ensure we don't generate the same ID again
	usedIDs = append(usedIDs, g.pendingIDs...)

	usedIDs = normalizeUsedIDs(usedIDs, args.minID, args.maxID)

	for range maxIDGenerateIterations {
		id, err := getIDCandidate(args.minID, args.maxID, usedIDs)
		if err != nil {
			return 0, err
		}

		available, err := args.isAvailableID(ctx, id)
		if err != nil {
			return 0, err
		}

		if !available {
			// If the GID is not available, try the next candidate
			usedIDs = append(usedIDs, id)
			log.Debugf(ctx, "%s %d is already used", args.idType, id)
			continue
		}

		g.pendingIDs = append(g.pendingIDs, id)
		return id, nil
	}

	return 0, fmt.Errorf("failed to find a valid %s for after %d attempts",
		args.idType, maxIDGenerateIterations)
}

// ClearPendingIDs clears the pending UIDs and GIDs.
// This function should be called once the generated IDs have been saved to the database.
func (g *IDGenerator) ClearPendingIDs() {
	g.pendingIDsMu.Lock()
	defer g.pendingIDsMu.Unlock()

	g.pendingIDs = nil
}

func getIDCandidate(minID, maxID uint32, usedIDs []uint32) (uint32, error) {
	if minID > maxID {
		return 0, errors.New("minID must be less than or equal to maxID")
	}

	// Find the highest used ID, if any
	var highestUsed uint32
	if minID > 0 {
		highestUsed = minID - 1 // No used IDs
	}
	if len(usedIDs) > 0 {
		highestUsed = max(highestUsed, usedIDs[len(usedIDs)-1])
	}

	// Try IDs above the highest used
	for id := highestUsed + 1; id <= maxID; id++ {
		if _, found := slices.BinarySearch(usedIDs, id); found {
			continue
		}
		return id, nil
	}

	// Fallback: try IDs from minID up to highestUsed
	for id := minID; id <= highestUsed && id <= maxID; id++ {
		if _, found := slices.BinarySearch(usedIDs, id); found {
			continue
		}
		return id, nil
	}

	return 0, errors.New("no available ID in range")
}

func (g *IDGenerator) isUIDAvailable(ctx context.Context, uid uint32) (bool, error) {
	lockedEntries := localentries.GetUserDBLocked(ctx)
	if unique, err := lockedEntries.IsUniqueUID(uid); !unique || err != nil {
		return false, err
	}

	return true, nil
}

func (g *IDGenerator) isGIDAvailable(ctx context.Context, gid uint32) (bool, error) {
	lockedEntries := localentries.GetUserDBLocked(ctx)
	if unique, err := lockedEntries.IsUniqueGID(gid); !unique || err != nil {
		return false, err
	}

	return true, nil
}

func (g *IDGenerator) getUsedIDs(ctx context.Context, owner IDOwner) ([]uint32, error) {
	usedUIDs, err := g.getUsedUIDs(ctx, owner)
	if err != nil {
		return nil, err
	}

	// For the user ID we also need to exclude all the GIDs, since the user
	// private group ID will match its own uid, so if we don't do this, we may
	// have a clash later on, when trying to add the group for this user.
	usedGIDs, err := g.getUsedGIDs(ctx, owner)
	if err != nil {
		return nil, err
	}

	return append(usedUIDs, usedGIDs...), nil
}

func (g *IDGenerator) getUsedUIDs(ctx context.Context, owner IDOwner) ([]uint32, error) {
	// Get the users from the authd database and pre-auth users.
	uids, err := owner.UsedUIDs()
	if err != nil {
		return nil, err
	}

	// Get the user entries from the passwd file. We don't use NSS here,
	// because for picking the next higher ID we only want to consider the users
	// in /etc/passwd and in the authd database, not from other sources like LDAP.
	userEntries, err := localentries.GetUserDBLocked(ctx).GetLocalUserEntries()
	if err != nil {
		return nil, err
	}
	for _, user := range userEntries {
		uids = append(uids, user.UID)
	}

	return uids, nil
}

func (g *IDGenerator) getUsedGIDs(ctx context.Context, owner IDOwner) ([]uint32, error) {
	gids, err := owner.UsedGIDs()
	if err != nil {
		return nil, err
	}

	// Get the group entries from the passwd file. We don't use NSS here,
	// because for picking the next higher ID we only want to consider the groups
	// in /etc/group and the users in /etc/group and in the authd database, not
	// from other sources like LDAP (in case merge method is used).
	groupEntries, err := localentries.GetUserDBLocked(ctx).GetLocalGroupEntries()
	if err != nil {
		return nil, err
	}
	for _, group := range groupEntries {
		gids = append(gids, group.GID)
	}

	// And include users GIDs too.
	userEntries, err := localentries.GetUserDBLocked(ctx).GetLocalUserEntries()
	if err != nil {
		return nil, err
	}
	for _, user := range userEntries {
		gids = append(gids, user.GID)
	}

	return gids, nil
}

func normalizeUsedIDs(usedIDs []uint32, minID, maxID uint32) []uint32 {
	// Sort usedIDs so we can binary search
	sort.Slice(usedIDs, func(i, j int) bool { return usedIDs[i] < usedIDs[j] })

	// Cut off usedIDs to the range we care about
	if len(usedIDs) > 0 && usedIDs[0] < minID {
		// Find the first ID >= minID
		firstIndex := slices.IndexFunc(usedIDs, func(id uint32) bool { return id >= minID })
		if firstIndex != -1 {
			// Slice usedIDs to start from the first ID >= minID
			usedIDs = usedIDs[firstIndex:]
		}
	}
	if len(usedIDs) > 0 && usedIDs[len(usedIDs)-1] > maxID {
		// Find the last ID <= maxID
		lastIndex := slices.IndexFunc(usedIDs, func(id uint32) bool { return id > maxID })
		if lastIndex != -1 {
			// Slice usedIDs to end at the last ID <= maxID
			usedIDs = usedIDs[:lastIndex]
		}
	}

	// Remove duplicates from usedIDs
	return slices.Compact(usedIDs)
}
