package cache

import (
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/ubuntu/decorate"
	"go.etcd.io/bbolt"
)

// DeleteUser removes the user from the database.
func (c *Cache) DeleteUser(uid int) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.db.Update(func(tx *bbolt.Tx) error {
		buckets, err := getAllBuckets(tx)
		if err != nil {
			return errors.Join(ErrNeedsClearing, err)
		}

		if err := deleteUser(buckets, uid); err != nil {
			if !errors.Is(err, NoDataFoundError{}) {
				return errors.Join(ErrNeedsClearing, err)
			}
			return err
		}
		return nil
	})
}

// deleteUserFromGroup removes the uid from the group.
// If the group is empty after the uid gets removed, the group is deleted from the database.
func deleteUserFromGroup(buckets map[string]bucketWithName, uid, gid int) error {
	groupToUsers, err := getFromBucket[groupToUsersDB](buckets[groupToUsersBucketName], gid)
	if err != nil && !errors.Is(err, NoDataFoundError{}) {
		return err
	}

	groupToUsers.UIDs = slices.DeleteFunc(groupToUsers.UIDs, func(id int) bool { return id == uid })
	if len(groupToUsers.UIDs) > 0 {
		// Update the group entry with the new list of UIDs
		updateBucket(buckets[groupToUsersBucketName], gid, groupToUsers)
		return nil
	}

	// We now need to delete this group with no remaining user.
	// We need the group.Name to delete from groupByName bucket.
	group, err := getFromBucket[GroupDB](buckets[groupByIDBucketName], gid)
	if err != nil {
		return err
	}

	// Delete group
	// Delete calls fail if the transaction is read only, so we should panic if this function is called in that context.
	if err = buckets[groupToUsersBucketName].Delete([]byte(strconv.Itoa(group.GID))); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	if err = buckets[groupByIDBucketName].Delete([]byte(strconv.Itoa(group.GID))); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	if err = buckets[groupByNameBucketName].Delete([]byte(group.Name)); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	return nil
}

// deleteUser removes the user from the database.
func deleteUser(buckets map[string]bucketWithName, uid int) (err error) {
	defer decorate.OnError(&err, "could not remove user %d from db", uid)

	u, err := getFromBucket[UserDB](buckets[userByIDBucketName], uid)
	if err != nil {
		return err
	}

	userToGroups, err := getFromBucket[userToGroupsDB](buckets[userToGroupsBucketName], uid)
	if err != nil {
		return err
	}
	for _, gid := range userToGroups.GIDs {
		if err := deleteUserFromGroup(buckets, uid, gid); err != nil {
			return err
		}
	}

	// Delete user
	// Delete calls fail if the transaction is read only, so we should panic if this function is called in that context.
	if err = buckets[userToGroupsBucketName].Delete([]byte(strconv.Itoa(u.UID))); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	if err = buckets[userByIDBucketName].Delete([]byte(strconv.Itoa(u.UID))); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	if err = buckets[userByNameBucketName].Delete([]byte(u.Name)); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	if err = buckets[userToBrokerBucketName].Delete([]byte(strconv.Itoa(u.UID))); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	return nil
}
