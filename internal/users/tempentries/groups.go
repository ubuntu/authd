package tempentries

import (
	"context"
	"fmt"
	"sync"

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

// generateGroupID registers a temporary group with a potential unique GID.
//
// Returns the generated GID .
func (r *temporaryGroupRecords) generateGroupID(name string) (gid uint32, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if there is already a temporary group with this name
	_, ok := r.gidByName[name]
	if ok {
		return 0, fmt.Errorf("group %q already exists", name)
	}

	// Generate a GID
	return r.idGenerator.GenerateGID()
}

func (r *temporaryGroupRecords) addTemporaryGroup(gid uint32, name string) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if g, ok := r.groups[gid]; ok {
		if g.gid == gid && g.name == name {
			r.gidByName[name] = gid
			log.Debugf(context.Background(),
				"Group %q with GID %d is already registered", name, gid)
			return nil
		}
		// This is really a programmer error if we get there, so... Let's just avoid it.
		return fmt.Errorf("A group with ID %d already exists, this should never happen", gid)
	}

	if _, ok := r.gidByName[name]; ok {
		log.Warningf(context.Background(),
			"A group entry for %q already exists, impossible to register the group")
		return fmt.Errorf("failed to register again group %q", name)
	}

	r.groups[gid] = groupRecord{name: name, gid: gid}
	r.gidByName[name] = gid

	log.Debugf(context.Background(), "Registered group %q with GID %d", name, gid)

	return nil
}

func (r *temporaryGroupRecords) deleteTemporaryGroup(gid uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	group, ok := r.groups[gid]
	if !ok {
		log.Warningf(context.Background(), "Can't delete temporary group with GID %d, it does not exist", gid)
		return
	}

	delete(r.groups, gid)
	delete(r.gidByName, group.name)

	log.Debugf(context.Background(), "Removed temporary record for group %q with GID %d", group.name, gid)
}
