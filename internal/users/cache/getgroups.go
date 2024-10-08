package cache

import (
	"encoding/json"
	"fmt"

	"go.etcd.io/bbolt"
)

type groupDB struct {
	Name string
	GID  uint32
}

// NewGroupDB creates a new GroupDB.
func NewGroupDB(name string, gid uint32, members []string) GroupDB {
	return GroupDB{
		Name:  name,
		GID:   gid,
		Users: members,
	}
}

// GroupByID returns a group matching this gid or an error if the database is corrupted or no entry was found.
func (c *Cache) GroupByID(gid uint32) (GroupDB, error) {
	return getGroup(c, groupByIDBucketName, gid)
}

// GroupByName returns a group matching a given name or an error if the database is corrupted or no entry was found.
func (c *Cache) GroupByName(name string) (GroupDB, error) {
	return getGroup(c, groupByNameBucketName, name)
}

// AllGroups returns all groups or an error if the database is corrupted.
func (c *Cache) AllGroups() (all []GroupDB, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	err = c.db.View(func(tx *bbolt.Tx) error {
		buckets, err := getAllBuckets(tx)
		if err != nil {
			return err
		}

		return buckets[groupByIDBucketName].ForEach(func(key, value []byte) error {
			var g groupDB
			if err := json.Unmarshal(value, &g); err != nil {
				return fmt.Errorf("can't unmarshal user in bucket %q for key %v: %v", userByIDBucketName, key, err)
			}

			// Get user names in the group.
			users, err := getUsersInGroup(buckets, g.GID)
			if err != nil {
				return err
			}

			all = append(all, NewGroupDB(g.Name, g.GID, users))
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return all, nil
}

// getGroup returns a group matching the key and its members or an error if the database is corrupted
// or no entry was found.
func getGroup[K uint32 | string](c *Cache, bucketName string, key K) (GroupDB, error) {
	var groupName string
	var gid uint32
	var users []string

	c.mu.RLock()
	defer c.mu.RUnlock()
	err := c.db.View(func(tx *bbolt.Tx) error {
		buckets, err := getAllBuckets(tx)
		if err != nil {
			return err
		}

		// Get id and name of the group.
		g, err := getFromBucket[groupDB](buckets[bucketName], key)
		if err != nil {
			return err
		}

		groupName = g.Name
		gid = g.GID

		// Get user names in the group.
		users, err = getUsersInGroup(buckets, gid)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return GroupDB{}, err
	}

	return NewGroupDB(groupName, gid, users), nil
}

// usersInGroup returns all user names in a given group. It returns an error if the database is corrupted.
func getUsersInGroup(buckets map[string]bucketWithName, gid uint32) (users []string, err error) {
	usersInGroup, err := getFromBucket[groupToUsersDB](buckets[groupToUsersBucketName], gid)
	if err != nil {
		return nil, err
	}

	for _, uid := range usersInGroup.UIDs {
		// we should always get an entry
		u, err := getFromBucket[UserDB](buckets[userByIDBucketName], uid)
		if err != nil {
			return nil, err
		}
		users = append(users, u.Name)
	}
	return users, nil
}
