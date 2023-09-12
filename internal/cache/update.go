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

// UpdateFromUserInfo inserts or updates user and group buckets from the user information.
func (c *Cache) UpdateFromUserInfo(u UserInfo) error {

	// create bucket contents dynamically
	gid := -1
	if len(u.Groups) > 0 {
		gid = u.Groups[0].GID
	}
	userDB := userDB{
		UserPasswdShadow: UserPasswdShadow{
			Name:           u.Name,
			UID:            u.UID,
			GID:            gid,
			Gecos:          u.Gecos,
			Dir:            u.Dir,
			Shell:          u.Shell,
			LastPwdChange:  -1,
			MaxPwdAge:      -1,
			PwdWarnPeriod:  -1,
			PwdInactivity:  -1,
			MinPwdAge:      -1,
			ExpirationDate: -1,
		},
		LastLogin: time.Now(),
	}

	var groupContents []groupDB
	for _, g := range u.Groups {
		groupContents = append(groupContents, groupDB(g))
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.db.Update(func(tx *bbolt.Tx) error {
		buckets, err := getAllBuckets(tx)
		if err != nil {
			c.requestClearDatabase()
			return err
		}

		previousGroupsForCurrentUser, err := getFromBucket[userToGroupsDB](buckets[userToGroupsBucketName], userDB.UID)
		// No data is valid and means this is the first insertion.
		if err != nil && !errors.Is(err, ErrNoDataFound{}) {
			c.requestClearDatabase()
			return err
		}

		// No groups were specified for this request.
		if userDB.GID == -1 {
			if len(previousGroupsForCurrentUser.GIDs) == 0 {
				return fmt.Errorf("no group provided for user %v (%v) and no previous record found", userDB.Name, userDB.UID)
			}

			for _, gid := range previousGroupsForCurrentUser.GIDs {
				g, err := getFromBucket[groupDB](buckets[groupByIDBucketName], gid)
				if err != nil {
					c.requestClearDatabase()
					return err
				}
				groupContents = append(groupContents, groupDB{
					Name: g.Name,
					GID:  g.GID,
				})
			}
			userDB.GID = groupContents[0].GID
		}

		/* 1. Handle user update */
		updateUser(buckets, userDB)

		/* 2. Handle groups update */
		updateGroups(buckets, groupContents)

		/* 3. Users and groups mapping buckets */
		if err := updateUsersAndGroups(buckets, userDB.UID, groupContents, previousGroupsForCurrentUser.GIDs); err != nil {
			c.requestClearDatabase()
			return err
		}

		return nil
	})

	return err
}

// updateUser updates both user buckets with userContent. It handles any potential login rename.
func updateUser(buckets map[string]bucketWithName, userContent userDB) {
	existingUser, err := getFromBucket[userDB](buckets[userByIDBucketName], userContent.UID)
	if err != nil && !errors.Is(err, ErrNoDataFound{}) {
		slog.Warn(fmt.Sprintf("Could not fetch previous record for user %v: %v", userContent.UID, err))
	}

	// If we updated the name, remove the previous login name
	if existingUser.Name != userContent.Name {
		_ = buckets[userByNameBucketName].Delete([]byte(existingUser.Name)) // No error as we are not in a RO transaction.
	}

	// Update user buckets
	updateBucket(buckets[userByIDBucketName], userContent.UID, userContent)
	updateBucket(buckets[userByNameBucketName], userContent.Name, userContent)
}

// updateUser updates both group buckets with groupContent. It handles any potential group rename.
func updateGroups(buckets map[string]bucketWithName, groupContents []groupDB) {
	for _, groupContent := range groupContents {
		existingGroup, err := getFromBucket[groupDB](buckets[groupByIDBucketName], groupContent.GID)
		if err != nil && !errors.Is(err, ErrNoDataFound{}) {
			slog.Warn(fmt.Sprintf("Could not fetch previous record for group %v: %v", groupContent.GID, err))
		}

		// If we updated the name, remove the previous group name
		if existingGroup.Name != groupContent.Name {
			_ = buckets[userByNameBucketName].Delete([]byte(existingGroup.Name)) // No error as we are not in a RO transaction.
		}

		// Update group buckets
		updateBucket(buckets[groupByIDBucketName], groupContent.GID, groupContent)
		updateBucket(buckets[groupByNameBucketName], groupContent.Name, groupContent)
	}
}

// updateUserAndGroups updates the pivot table for user to groups and group to users. It handles any update
// to groups uid is not part of anymore.
func updateUsersAndGroups(buckets map[string]bucketWithName, uid int, groupContents []groupDB, previousGIDs []int) error {
	var currentGIDs []int
	for _, groupContent := range groupContents {
		currentGIDs = append(currentGIDs, groupContent.GID)
		grpToUsers, err := getFromBucket[groupToUsersDB](buckets[groupToUsersBucketName], groupContent.GID)
		// No data is valid and means that this is the first time we record it.
		if err != nil && !errors.Is(err, ErrNoDataFound{}) {
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
		grpToUsers, err := getFromBucket[groupToUsersDB](buckets[groupToUsersBucketName], previousGID)
		// No data means the database was corrupted but we can forgive this (exist in previous user gid but not on system).
		if err != nil && !errors.Is(err, ErrNoDataFound{}) {
			return err
		}

		grpToUsers.UIDs = slices.DeleteFunc(grpToUsers.UIDs, func(eUID int) bool { return eUID == uid })

		if len(grpToUsers.UIDs) != 0 {
			updateBucket(buckets[groupToUsersBucketName], previousGID, grpToUsers)
			continue
		}

		// It means we need to delete this group with no remaining user.
		// It’s thus not in userToGroups bucket.
		group, err := getFromBucket[groupDB](buckets[groupByIDBucketName], previousGID)
		if err != nil {
			return err
		}

		// Delete can't error out as we are not in a RO transaction.
		_ = buckets[groupByIDBucketName].Delete([]byte(strconv.Itoa(previousGID)))
		_ = buckets[groupByNameBucketName].Delete([]byte(group.Name))
		_ = buckets[groupToUsersBucketName].Delete([]byte(strconv.Itoa(previousGID)))

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
