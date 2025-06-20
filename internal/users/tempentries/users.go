package tempentries

import (
	"context"
	"sync"

	"github.com/ubuntu/authd/log"
)

type referencedUserName struct {
	refCount uint64
	uid      uint32
}

type idTracker struct {
	mu        sync.Mutex
	ids       map[uint32]struct{}
	userNames map[string]*referencedUserName
}

func newIDTracker() *idTracker {
	return &idTracker{
		ids:       make(map[uint32]struct{}),
		userNames: make(map[string]*referencedUserName),
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

func (r *idTracker) trackUser(name string, uid uint32) (tracked bool, currentID uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if current, ok := r.userNames[name]; ok {
		log.Debugf(context.Background(),
			"Not tracking user name %q for UID %d, already tracked as %d",
			name, uid, current.uid)
		if current.uid != uid {
			return false, current.uid
		}

		current.refCount++
		return true, current.uid
	}

	log.Debugf(context.Background(), "Tracking user name %q for UID %d", name, uid)
	r.userNames[name] = &referencedUserName{uid: uid, refCount: 1}
	return true, uid
}

func (r *idTracker) forgetUser(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.userNames[name]

	if !ok {
		log.Debugf(context.Background(), "No tracked user for %q", name)
		return
	}

	if current.refCount > 1 {
		current.refCount--
		return
	}

	log.Debugf(context.Background(),
		"Forgetting tracked user name %q for UID %d", name, current.uid)

	delete(r.userNames, name)
}
