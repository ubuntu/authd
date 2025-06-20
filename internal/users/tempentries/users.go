package tempentries

import (
	"context"
	"sync"

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
