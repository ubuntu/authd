package tempentries

import (
	"github.com/ubuntu/authd/internal/users/db"
	"github.com/ubuntu/authd/internal/users/types"
)

// GroupByID returns the group information for the given group ID.
func (r *temporaryGroupRecords) GroupByID(gid uint32) (types.GroupEntry, error) {
	group, ok := r.groups[gid]
	if !ok {
		return types.GroupEntry{}, db.NewGIDNotFoundError(gid)
	}

	return groupEntry(group), nil
}

func groupEntry(group groupRecord) types.GroupEntry {
	return types.GroupEntry{Name: group.name, GID: group.gid}
}

// GroupByName returns the group information for the given group name.
func (r *temporaryGroupRecords) GroupByName(name string) (types.GroupEntry, error) {
	gid, ok := r.gidByName[name]
	if !ok {
		return types.GroupEntry{}, db.NewGroupNotFoundError(name)
	}

	return r.GroupByID(gid)
}
