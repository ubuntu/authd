package tempentries

import (
	"context"
	"fmt"
	"sync"

	"github.com/ubuntu/authd/log"
)

type groupRecord struct {
	refCount uint64
	name     string
	gid      uint32
}

type temporaryGroupRecords struct {
	idGenerator IDGenerator
	mu          sync.Mutex
	groups      map[uint32]*groupRecord
	gidByName   map[string]uint32
}

func newTemporaryGroupRecords(idGenerator IDGenerator) *temporaryGroupRecords {
	return &temporaryGroupRecords{
		idGenerator: idGenerator,
		groups:      make(map[uint32]*groupRecord),
		gidByName:   make(map[string]uint32),
	}
}

// generateGroupID generates a potential unique GID.
//
// Returns the generated GID .
func (r *temporaryGroupRecords) generateGroupID() (gid uint32, err error) {
	// Generate a GID
	return r.idGenerator.GenerateGID()
}

// getTemporaryGroup gets the GID of a temporary group with the given name,
// if any, increasing its reference count.
//
// This must be called with the group mutex locked.
func (r *temporaryGroupRecords) getTemporaryGroup(name string) (gid uint32) {
	gid, ok := r.gidByName[name]
	if !ok {
		return 0
	}

	group := r.groups[gid]
	group.refCount++

	log.Debugf(context.Background(), "Reusing GID %d for group %q", gid, group.name)

	return group.gid
}

// Adds a temporary group.
//
// This must be called with the group mutex locked.
func (r *temporaryGroupRecords) addTemporaryGroup(gid uint32, name string) {
	// log.Debugf(nil, "Adding group %d (%q)", gid, name)
	if oldGroup, ok := r.groups[gid]; ok {
		panic(fmt.Sprintf("group ID %d is already registered for %q, cannot register %q",
			gid, oldGroup.name, name))
	}
	if oldGID, ok := r.gidByName[name]; ok {
		panic(fmt.Sprintf("group %q has already GID %d, cannot use %d", name, oldGID, gid))
	}

	r.groups[gid] = &groupRecord{name: name, gid: gid, refCount: 1}
	r.gidByName[name] = gid

	log.Debugf(context.Background(), "Registered group %q with GID %d", name, gid)
}

// releaseTemporaryGroup releases a temporary group reference, it returns
// whether the last reference has been dropped.
//
// This must be called with the group mutex locked.
func (r *temporaryGroupRecords) releaseTemporaryGroup(gid uint32) bool {
	group, ok := r.groups[gid]
	if !ok {
		log.Warningf(context.Background(), "Can't delete temporary group with GID %d, it does not exist", gid)
		return false
	}

	if group.refCount > 1 {
		log.Debugf(context.Background(), "Removed reference on temporary record for group %q with GID %d",
			group.name, gid)
		group.refCount--
		return false
	}

	delete(r.groups, gid)
	delete(r.gidByName, group.name)

	log.Debugf(context.Background(), "Removed temporary record for group %q with GID %d", group.name, gid)
	return true
}
