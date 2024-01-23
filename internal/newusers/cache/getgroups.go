package cache

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
)

type groupDB struct {
	Name string
	GID  int
}

// NewGroupDB creates a new GroupDB.
func NewGroupDB(name string, gid int, members []string) GroupDB {
	return GroupDB{
		Name:  name,
		GID:   gid,
		Users: members,
	}
}

// GroupByID returns a group matching this gid or an error if the database is corrupted or no entry was found.
// Upon corruption, clearing the database is requested.
func (c *Cache) GroupByID(gid int) (GroupDB, error) {
	return getGroup(c, groupByIDBucketName, gid)
}

// GroupByName returns a group matching a given name or an error if the database is corrupted or no entry was found.
// Upon corruption, clearing the database is requested.
func (c *Cache) GroupByName(name string) (GroupDB, error) {
	return getGroup(c, groupByNameBucketName, name)
}

// AllGroups returns all groups or an error if the database is corrupted.
// Upon corruption, clearing the database is requested.
func (c *Cache) AllGroups() (all []GroupDB, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	err = c.db.View(func(tx *bbolt.Tx) error {
		buckets, err := getAllBuckets(tx)
		if err != nil {
			return errors.Join(ErrNeedsClearing, err)
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
		return nil, errors.Join(ErrNeedsClearing, err)
	}

	return all, nil
}

// getGroup returns a group matching the key and its members or an error if the database is corrupted
// or no entry was found. Upon corruption, clearing the database is requested.
func getGroup[K int | string](c *Cache, bucketName string, key K) (GroupDB, error) {
	var groupName string
	var gid int
	var users []string

	c.mu.RLock()
	defer c.mu.RUnlock()
	err := c.db.View(func(tx *bbolt.Tx) error {
		buckets, err := getAllBuckets(tx)
		if err != nil {
			return errors.Join(ErrNeedsClearing, err)
		}

		// Get id and name of the group.
		g, err := getFromBucket[groupDB](buckets[bucketName], key)
		if err != nil {
			// no entry is valid, no need to clean the database but return the error.
			if !errors.Is(err, NoDataFoundError{}) {
				err = errors.Join(ErrNeedsClearing, err)
			}
			return err
		}

		groupName = g.Name
		gid = g.GID

		// Get user names in the group.
		users, err = getUsersInGroup(buckets, gid)
		if err != nil {
			return errors.Join(ErrNeedsClearing, err)
		}

		return nil
	})

	if err != nil {
		return GroupDB{}, err
	}

	return NewGroupDB(groupName, gid, users), nil
}

// usersInGroup returns all user names in a given group. It returns an error if the database is corrupted.
func getUsersInGroup(buckets map[string]bucketWithName, gid int) (users []string, err error) {
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
