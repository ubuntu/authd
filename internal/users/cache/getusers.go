package cache

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

// userDB is the struct stored in json format in the bucket.
//
// It prevents leaking of lastLogin, which is only relevant to the cache.
type userDB struct {
	UserDB
	LastLogin time.Time
}

// NewUserDB creates a new UserDB.
func NewUserDB(name string, uid, gid uint32, gecos, dir, shell string) UserDB {
	return UserDB{
		Name:           name,
		UID:            uid,
		GID:            gid,
		Gecos:          gecos,
		Dir:            dir,
		Shell:          shell,
		LastPwdChange:  -1,
		MaxPwdAge:      -1,
		PwdWarnPeriod:  -1,
		PwdInactivity:  -1,
		MinPwdAge:      -1,
		ExpirationDate: -1,
	}
}

// UserByID returns a user matching this uid or an error if the database is corrupted or no entry was found.
func (c *Cache) UserByID(uid uint32) (UserDB, error) {
	u, err := getUser(c, userByIDBucketName, uid)
	return u.UserDB, err
}

// UserByName returns a user matching this name or an error if the database is corrupted or no entry was found.
func (c *Cache) UserByName(name string) (UserDB, error) {
	// authd uses lowercase usernames
	name = strings.ToLower(name)
	u, err := getUser(c, userByNameBucketName, name)
	return u.UserDB, err
}

// AllUsers returns all users or an error if the database is corrupted.
func (c *Cache) AllUsers() (users []UserDB, err error) {
	userRecords, err := c.allUsers()
	if err != nil {
		return nil, err
	}
	for _, u := range userRecords {
		users = append(users, u.UserDB)
	}
	return users, nil
}

// allUsers is an internal function to get all user records from the database (including lastLogin).
func (c *Cache) allUsers() (users []userDB, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	err = c.db.View(func(tx *bbolt.Tx) error {
		bucket, err := getBucket(tx, userByIDBucketName)
		if err != nil {
			return err
		}

		return bucket.ForEach(func(key, value []byte) error {
			var e userDB
			if err := json.Unmarshal(value, &e); err != nil {
				return fmt.Errorf("can't unmarshal user in bucket %q for key %v: %v", userByIDBucketName, key, err)
			}
			users = append(users, e)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return users, nil
}

// getUser returns an user matching the key or an error if the database is corrupted or no entry was found.
func getUser[K uint32 | string](c *Cache, bucketName string, key K) (u userDB, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	err = c.db.View(func(tx *bbolt.Tx) error {
		bucket, err := getBucket(tx, bucketName)
		if err != nil {
			return err
		}

		u, err = getFromBucket[userDB](bucket, key)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return userDB{}, err
	}

	return u, nil
}
