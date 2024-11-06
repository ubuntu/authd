package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/ubuntu/authd/internal/log"
	"github.com/ubuntu/decorate"
	"go.etcd.io/bbolt"
)

// DeleteUser removes the user from the database.
func (c *Cache) DeleteUser(uid uint32) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.db.Update(func(tx *bbolt.Tx) error {
		buckets, err := getAllBuckets(tx)
		if err != nil {
			return err
		}

		if err := deleteUser(buckets, uid); err != nil {
			return err
		}
		return nil
	})
}

// deleteUserFromGroup removes the uid from the group.
// If the group is empty after the uid gets removed, the group is deleted from the database.
func deleteUserFromGroup(buckets map[string]bucketWithName, uid, gid uint32) error {
	groupToUsers, err := getFromBucket[groupToUsersDB](buckets[groupToUsersBucketName], gid)
	if err != nil && !errors.Is(err, NoDataFoundError{}) {
		return err
	}

	groupToUsers.UIDs = slices.DeleteFunc(groupToUsers.UIDs, func(id uint32) bool { return id == uid })
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

	gidKey := []byte(strconv.FormatUint(uint64(gid), 10))
	// Delete group
	// Delete calls fail if the transaction is read only, so we should panic if this function is called in that context.
	if err = buckets[groupToUsersBucketName].Delete(gidKey); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	if err = buckets[groupByIDBucketName].Delete(gidKey); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	if err = buckets[groupByNameBucketName].Delete([]byte(group.Name)); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	return nil
}

// deleteUser removes the user from the database.
func deleteUser(buckets map[string]bucketWithName, uid uint32) (err error) {
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

	uidKey := []byte(strconv.FormatUint(uint64(u.UID), 10))

	// Delete user
	// Delete calls fail if the transaction is read only, so we should panic if this function is called in that context.
	if err = buckets[userToGroupsBucketName].Delete(uidKey); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	if err = buckets[userByIDBucketName].Delete(uidKey); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	if err = buckets[userByNameBucketName].Delete([]byte(u.Name)); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	if err = buckets[userToBrokerBucketName].Delete(uidKey); err != nil {
		panic(fmt.Sprintf("programming error: delete is not allowed in a RO transaction: %v", err))
	}
	return nil
}

// deleteOrphanedUsers removes users from the UserByID bucket that are not in the UserByName bucket.
func deleteOrphanedUsers(db *bbolt.DB) error {
	log.Debug(context.TODO(), "Cleaning up orphaned user records")

	err := db.Update(func(tx *bbolt.Tx) error {
		buckets, err := getAllBuckets(tx)
		if err != nil {
			return err
		}

		return buckets[userByIDBucketName].ForEach(func(k, v []byte) error {
			var user UserDB
			if err := json.Unmarshal(v, &user); err != nil {
				log.Warningf(context.TODO(), "Error loading user record {%s: %s}: %v", k, v, err)
				return nil
			}

			user2, err := getFromBucket[userDB](buckets[userByNameBucketName], user.Name)
			if err != nil && !errors.Is(err, NoDataFoundError{}) {
				log.Warningf(context.TODO(), "Error loading user record %q: %v", user.Name, err)
				return nil
			}
			if errors.Is(err, NoDataFoundError{}) || user2.UID != user.UID {
				log.Warningf(context.TODO(), "Removing orphaned user record %q with UID %d", user.Name, user.UID)
				return deleteOrphanedUser(buckets, user.UID)
			}

			return nil
		})
	})
	if err != nil {
		return err
	}

	log.Debug(context.TODO(), "Done cleaning up orphaned user records")
	return nil
}

// deleteOrphanedUser removes all user records with the given UID from the buckets, except the UserByName bucket.
func deleteOrphanedUser(buckets map[string]bucketWithName, uid uint32) (err error) {
	uidKey := []byte(strconv.FormatUint(uint64(uid), 10))

	if err := buckets[userByIDBucketName].Delete(uidKey); err != nil {
		return fmt.Errorf("can't delete user with UID %d from userByID bucket: %v", uid, err)
	}

	groups, err := getFromBucket[userToGroupsDB](buckets[userToGroupsBucketName], uid)
	if err != nil && !errors.Is(err, NoDataFoundError{}) {
		return err
	}
	for _, gid := range groups.GIDs {
		if err := deleteUserFromGroup(buckets, uid, gid); err != nil {
			return err
		}
	}

	if err := buckets[userToGroupsBucketName].Delete(uidKey); err != nil {
		return fmt.Errorf("can't delete user with UID %d from userToGroups bucket: %v", uid, err)
	}
	if err := buckets[userToBrokerBucketName].Delete(uidKey); err != nil {
		return fmt.Errorf("can't delete user with UID %d from userToBroker bucket: %v", uid, err)
	}

	return nil
}
