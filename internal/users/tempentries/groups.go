package tempentries

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"

	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/localentries"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
)

type groupRecord struct {
	name   string
	gid    uint32
	passwd string
}

type temporaryGroupRecords struct {
	idGenerator IDGenerator
	registerMu  sync.Mutex
	rwMu        sync.RWMutex
	groups      map[uint32]groupRecord
	gidByName   map[string]uint32
}

func newTemporaryGroupRecords(idGenerator IDGenerator) *temporaryGroupRecords {
	return &temporaryGroupRecords{
		idGenerator: idGenerator,
		registerMu:  sync.Mutex{},
		rwMu:        sync.RWMutex{},
		groups:      make(map[uint32]groupRecord),
		gidByName:   make(map[string]uint32),
	}
}

// GroupByID returns the group information for the given group ID.
func (r *temporaryGroupRecords) GroupByID(gid uint32) (types.GroupEntry, error) {
	r.rwMu.RLock()
	defer r.rwMu.RUnlock()

	group, ok := r.groups[gid]
	if !ok {
		return types.GroupEntry{}, db.NewGIDNotFoundError(gid)
	}

	return groupEntry(group), nil
}

// GroupByName returns the group information for the given group name.
func (r *temporaryGroupRecords) GroupByName(name string) (types.GroupEntry, error) {
	r.rwMu.RLock()
	defer r.rwMu.RUnlock()

	gid, ok := r.gidByName[name]
	if !ok {
		return types.GroupEntry{}, db.NewGroupNotFoundError(name)
	}

	return r.GroupByID(gid)
}

func groupEntry(group groupRecord) types.GroupEntry {
	return types.GroupEntry{Name: group.name, GID: group.gid, Passwd: group.passwd}
}

// RegisterGroup registers a temporary group with a unique GID in our NSS handler (in memory, not in the database).
//
// Returns the generated GID and a cleanup function that should be called to remove the temporary group once the group
// was added to the database.
func (r *temporaryGroupRecords) RegisterGroup(name string) (gid uint32, cleanup func(), err error) {
	r.registerMu.Lock()
	defer r.registerMu.Unlock()

	// Check if there is already a temporary group with this name
	_, err = r.GroupByName(name)
	if err != nil && !errors.Is(err, NoDataFoundError{}) {
		return 0, nil, fmt.Errorf("could not check if temporary group %q already exists: %w", name, err)
	}
	if err == nil {
		return 0, nil, fmt.Errorf("group %q already exists", name)
	}

	// Generate a GID until we find a unique one
	for {
		gid, err = r.idGenerator.GenerateGID()
		if err != nil {
			return 0, nil, err
		}

		// To avoid races where a group with this GID is created by some NSS source after we checked, we register this
		// GID in our NSS handler and then check if another group with the same GID exists in the system. This way we
		// can guarantee that the GID is unique, under the assumption that other NSS sources don't add groups with a GID
		// that we already registered (if they do, there's nothing we can do about it).
		var tmpID string
		tmpID, cleanup, err = r.addTemporaryGroup(gid, name)
		if err != nil {
			return 0, nil, fmt.Errorf("could not register temporary group: %w", err)
		}

		unique, err := r.uniqueNameAndGID(name, gid, tmpID)
		if err != nil {
			cleanup()
			return 0, nil, fmt.Errorf("could not check if GID %d is unique: %w", gid, err)
		}
		if unique {
			break
		}

		// If the GID is not unique, remove the temporary group and generate a new one in the next iteration.
		cleanup()
	}

	log.Debugf(context.Background(), "Registered group %q with GID %d", name, gid)
	return gid, cleanup, nil
}

func (r *temporaryGroupRecords) uniqueNameAndGID(name string, gid uint32, tmpID string) (bool, error) {
	entries, err := localentries.GetGroupEntries()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.Name == name && entry.Passwd != tmpID {
			// A group with the same name already exists, we can't register this temporary group.
			log.Debugf(context.Background(), "Name %q already in use by GID %d", name, entry.GID)
			return false, fmt.Errorf("group %q already exists", name)
		}

		if entry.GID == gid && entry.Passwd != tmpID {
			log.Debugf(context.Background(), "GID %d already in use by group %q, generating a new one", gid, entry.Name)
			return false, nil
		}
	}

	return true, nil
}

func (r *temporaryGroupRecords) addTemporaryGroup(gid uint32, name string) (tmpID string, cleanup func(), err error) {
	r.rwMu.Lock()
	defer r.rwMu.Unlock()

	// Generate a 64 character (32 bytes in hex) random ID which we store in the passwd field of the temporary group
	// record to be able to identify it in isUniqueGID.
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate random name: %w", err)
	}
	tmpID = fmt.Sprintf("authd-temp-group-%x", bytes)

	r.groups[gid] = groupRecord{name: name, gid: gid, passwd: tmpID}
	r.gidByName[name] = gid

	cleanup = func() { r.deleteTemporaryGroup(gid) }

	return tmpID, cleanup, nil
}

func (r *temporaryGroupRecords) deleteTemporaryGroup(gid uint32) {
	r.rwMu.Lock()
	defer r.rwMu.Unlock()

	group, ok := r.groups[gid]
	if !ok {
		log.Warningf(context.Background(), "Can't delete temporary group with GID %d, it does not exist", gid)
		return
	}

	delete(r.groups, gid)
	delete(r.gidByName, group.name)

	log.Debugf(context.Background(), "Removed temporary record for group %q with GID %d", group.name, gid)
}
