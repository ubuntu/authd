package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"go.etcd.io/bbolt"
)

// UpdateUserEntry inserts or updates user and group buckets from the user information.
func (c *Cache) UpdateUserEntry(usr UserDB, groupContents []GroupDB) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	userDB := userDB{
		UserDB:    usr,
		LastLogin: time.Now(),
	}

	err := c.db.Update(func(tx *bbolt.Tx) error {
		buckets, err := getAllBuckets(tx)
		if err != nil {
			return errors.Join(ErrNeedsClearing, err)
		}

		previousGroupsForCurrentUser, err := getFromBucket[userToGroupsDB](buckets[userToGroupsBucketName], userDB.UID)
		// No data is valid and means this is the first insertion.
		if err != nil && !errors.Is(err, NoDataFoundError{}) {
			return errors.Join(ErrNeedsClearing, err)
		}

		/* 1. Handle user update */
		updateUser(buckets, userDB)

		/* 2. Handle groups update */
		updateGroups(buckets, groupContents)

		/* 3. Users and groups mapping buckets */
		if err := updateUsersAndGroups(buckets, userDB.UID, groupContents, previousGroupsForCurrentUser.GIDs); err != nil {
			return errors.Join(ErrNeedsClearing, err)
		}

		return nil
	})

	return err
}

// updateUser updates both user buckets with userContent. It handles any potential login rename.
func updateUser(buckets map[string]bucketWithName, userContent userDB) {
	existingUser, err := getFromBucket[userDB](buckets[userByIDBucketName], userContent.UID)
	if err != nil && !errors.Is(err, NoDataFoundError{}) {
		slog.Warn(fmt.Sprintf("Could not fetch previous record for user %v: %v", userContent.UID, err))
	}

	// If we updated the name, remove the previous login name
	if existingUser.Name != userContent.Name {
		_ = buckets[userByNameBucketName].Delete([]byte(existingUser.Name)) // No error as we are not in a RO transaction.
	}

	// Ensure that we use the same homedir as the one we have in cache.
	if existingUser.Dir != "" && existingUser.Dir != userContent.Dir {
		slog.Warn(fmt.Sprintf("User %q already has a homedir. The existing %q one will be kept instead of %q", userContent.Name, existingUser.Dir, userContent.Dir))
		userContent.Dir = existingUser.Dir
	}

	// Update user buckets
	updateBucket(buckets[userByIDBucketName], userContent.UID, userContent)
	updateBucket(buckets[userByNameBucketName], userContent.Name, userContent)
}

// updateUser updates both group buckets with groupContent. It handles any potential group rename.
func updateGroups(buckets map[string]bucketWithName, groupContents []GroupDB) {
	for _, groupContent := range groupContents {
		existingGroup, err := getFromBucket[groupDB](buckets[groupByIDBucketName], groupContent.GID)
		if err != nil && !errors.Is(err, NoDataFoundError{}) {
			slog.Warn(fmt.Sprintf("Could not fetch previous record for group %v: %v", groupContent.GID, err))
		}

		// If we updated the name, remove the previous group name
		if existingGroup.Name != groupContent.Name {
			_ = buckets[groupByNameBucketName].Delete([]byte(existingGroup.Name)) // No error as we are not in a RO transaction.
		}

		// Update group buckets
		updateBucket(buckets[groupByIDBucketName], groupContent.GID, groupDB{Name: groupContent.Name, GID: groupContent.GID})
		updateBucket(buckets[groupByNameBucketName], groupContent.Name, groupDB{Name: groupContent.Name, GID: groupContent.GID})
	}
}

// updateUserAndGroups updates the pivot table for user to groups and group to users. It handles any update
// to groups uid is not part of anymore.
func updateUsersAndGroups(buckets map[string]bucketWithName, uid int, groupContents []GroupDB, previousGIDs []int) error {
	var currentGIDs []int
	for _, groupContent := range groupContents {
		currentGIDs = append(currentGIDs, groupContent.GID)
		grpToUsers, err := getFromBucket[groupToUsersDB](buckets[groupToUsersBucketName], groupContent.GID)
		// No data is valid and means that this is the first time we record it.
		if err != nil && !errors.Is(err, NoDataFoundError{}) {
			return err
		}

		grpToUsers.GID = groupContent.GID
		if !slices.Contains(grpToUsers.UIDs, uid) {
			grpToUsers.UIDs = append(grpToUsers.UIDs, uid)
		}
		updateBucket(buckets[groupToUsersBucketName], groupContent.GID, grpToUsers)
	}
	updateBucket(buckets[userToGroupsBucketName], uid, userToGroupsDB{UID: uid, GIDs: currentGIDs})

	// Remove UID from any groups this user is not part of anymore.
	for _, previousGID := range previousGIDs {
		if slices.Contains(currentGIDs, previousGID) {
			continue
		}
		if err := deleteUserFromGroup(buckets, uid, previousGID); err != nil {
			return err
		}
	}

	return nil
}

// updateBucket is a generic function to update any bucket. It panics if we call it in RO transaction.
func updateBucket[K int | string](bucket bucketWithName, key K, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("programming error: %v is not a valid json", err))
	}

	// TODO: switch to https://github.com/golang/go/issues/45380 if accepted.
	var k []byte
	switch v := any(key).(type) {
	case int:
		k = []byte(strconv.Itoa(v))
	case string:
		k = []byte(v)
	default:
		panic(fmt.Sprintf("unhandled type: %T", key))
	}

	if err = bucket.Put(k, data); err != nil {
		panic(fmt.Sprintf("programming error: Put is not executed in a RW transaction: %v", err))
	}
}

// UpdateBrokerForUser updates the last broker the user successfully authenticated with.
func (c *Cache) UpdateBrokerForUser(username, brokerID string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	u, err := c.UserByName(username)
	if err != nil {
		return err
	}

	err = c.db.Update(func(tx *bbolt.Tx) error {
		bucket, err := getBucket(tx, userToBrokerBucketName)
		if err != nil {
			return errors.Join(ErrNeedsClearing, err)
		}
		updateBucket(bucket, u.UID, brokerID)
		return nil
	})

	return err
}
