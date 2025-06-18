package tempentries

import (
	"context"
	"fmt"
	"sync"

	"github.com/ubuntu/authd/internal/users/types"
	"github.com/ubuntu/authd/log"
)

type idTracker struct {
	mu        sync.Mutex
	ids       map[uint32]struct{}
	userNames map[string]uint32
}

func newIDTracker() *idTracker {
	return &idTracker{
		ids:       make(map[uint32]struct{}),
		userNames: make(map[string]uint32),
	}
}

func (r *idTracker) trackID(id uint32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, ok := r.ids[id]
	if ok {
		log.Debugf(context.Background(), "Not tracking ID %d, already tracked", id)
		return false
	}

	log.Debugf(context.Background(), "Tracking ID %d", id)
	r.ids[id] = struct{}{}
	return true
}

func (r *idTracker) forgetID(id uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if log.IsLevelEnabled(log.DebugLevel) {
		_, ok := r.ids[id]
		log.Debugf(context.Background(),
			"Forgetting tracked ID %d: %v", id, ok)
	}

	delete(r.ids, id)
}

func (r *idTracker) trackUser(name string, id uint32) (tracked bool, currentID uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if currentID, ok := r.userNames[name]; ok {
		log.Debugf(context.Background(),
			"Not tracking user name %q for UID %d, already tracked as %d", name, id, currentID)
		return currentID == id, currentID
	}

	log.Debugf(context.Background(), "Tracking user name %q for UID %d", name, id)
	r.userNames[name] = id
	return true, id
}

func (r *idTracker) forgetUser(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if log.IsLevelEnabled(log.DebugLevel) {
		id, ok := r.userNames[name]
		log.Debugf(context.Background(),
			"Forgetting tracked user name %q for UID %d: %v", name, id, ok)
	}
	delete(r.userNames, name)
}

// uniqueNameAndUID returns true if the given UID is unique in the system. It returns false if the UID is already assigned to
// a user by any NSS source.
func uniqueNameAndUID(name string, uid uint32, passwdEntries []types.UserEntry, groupEntries []types.GroupEntry) (bool, error) {
	for _, entry := range passwdEntries {
		if entry.Name == name && entry.UID != uid {
			// A user with the same name already exists, we can't register this temporary user.
			log.Debugf(context.Background(), "Name %q already in use by UID %d", name, entry.UID)
			return false, fmt.Errorf("user %q already exists", name)
		}

		if entry.UID == uid {
			log.Debugf(context.Background(), "UID %d already in use by user %q, generating a new one", uid, entry.Name)
			return false, nil
		}
	}

	for _, group := range groupEntries {
		if group.GID == uid {
			// A group with the same ID already exists, so we can't use that ID as the GID of the temporary user.
			log.Debugf(context.Background(), "ID %d already in use by group %q", uid, group.Name)
			return false, fmt.Errorf("group with GID %d already exists", uid)
		}
	}

	return true, nil
}
