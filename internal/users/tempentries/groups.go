package tempentries

import (
	"context"
	"fmt"
	"sync"

	"github.com/ubuntu/authd/internal/users/localentries"
	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
)

type groupRecord struct {
	name string
	gid  uint32
}

type temporaryGroupRecords struct {
	idGenerator IDGenerator
	mu          sync.Mutex
	groups      map[uint32]groupRecord
	gidByName   map[string]uint32
}

func newTemporaryGroupRecords(idGenerator IDGenerator) *temporaryGroupRecords {
	return &temporaryGroupRecords{
		idGenerator: idGenerator,
		groups:      make(map[uint32]groupRecord),
		gidByName:   make(map[string]uint32),
	}
}

// registerGroup registers a temporary group with a unique GID in our NSS handler (in memory, not in the database).
//
// Returns the generated GID and a cleanup function that should be called to remove the temporary group.
func (r *temporaryGroupRecords) registerGroup(name string) (gid uint32, cleanup func(), err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if there is already a temporary group with this name
	_, ok := r.gidByName[name]
	if ok {
		return 0, nil, fmt.Errorf("group %q already exists", name)
	}

	groupEntries, err := localentries.GetGroupEntries()
	if err != nil {
		return 0, nil, fmt.Errorf("could not register group, failed to get group entries: %w", err)
	}

	// Generate a GID until we find a unique one
	for {
		gid, err = r.idGenerator.GenerateGID()
		if err != nil {
			return 0, nil, err
		}

		unique, err := r.uniqueNameAndGID(name, gid, groupEntries)
		if err != nil {
			return 0, nil, fmt.Errorf("could not check if GID %d is unique: %w", gid, err)
		}
		if !unique {
			// If the GID is not unique, generate a new one in the next iteration.
			continue
		}

		cleanup = r.addTemporaryGroup(gid, name)
		log.Debugf(context.Background(), "Registered group %q with GID %d", name, gid)
		return gid, cleanup, nil
	}
}

func (r *temporaryGroupRecords) uniqueNameAndGID(name string, gid uint32, groupEntries []types.GroupEntry) (bool, error) {
	if _, ok := r.groups[gid]; ok {
		return false, nil
	}

	for _, entry := range groupEntries {
		if entry.Name == name {
			// A group with the same name already exists, we can't register this temporary group.
			log.Debugf(context.Background(), "Name %q already in use by GID %d", name, entry.GID)
			return false, fmt.Errorf("group %q already exists", name)
		}

		if entry.GID == gid {
			log.Debugf(context.Background(), "GID %d already in use by group %q, generating a new one", gid, entry.Name)
			return false, nil
		}
	}

	return true, nil
}

func (r *temporaryGroupRecords) addTemporaryGroup(gid uint32, name string) (cleanup func()) {
	r.groups[gid] = groupRecord{name: name, gid: gid}
	r.gidByName[name] = gid

	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		r.deleteTemporaryGroup(gid)
	}
}

func (r *temporaryGroupRecords) deleteTemporaryGroup(gid uint32) {
	group, ok := r.groups[gid]
	if !ok {
		log.Warningf(context.Background(), "Can't delete temporary group with GID %d, it does not exist", gid)
		return
	}

	delete(r.groups, gid)
	delete(r.gidByName, group.name)

	log.Debugf(context.Background(), "Removed temporary record for group %q with GID %d", group.name, gid)
}
