package cache

import (
	"encoding/json"
	"errors"
	"fmt"
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
func NewUserDB(name string, uid, gid int, gecos, dir, shell string) UserDB {
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
// Upon corruption, clearing the database is requested.
func (c *Cache) UserByID(uid int) (UserDB, error) {
	u, err := getUser(c, userByIDBucketName, uid)
	return u.UserDB, err
}

// UserByName returns a user matching this name or an error if the database is corrupted or no entry was found.
// Upon corruption, clearing the database is requested.
func (c *Cache) UserByName(name string) (UserDB, error) {
	u, err := getUser(c, userByNameBucketName, name)
	return u.UserDB, err
}

// AllUsers returns all users or an error if the database is corrupted.
// Upon corruption, clearing the database is requested.
func (c *Cache) AllUsers() (all []UserDB, err error) {
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
			all = append(all, e.UserDB)
			return nil
		})
	})

	if err != nil {
		return nil, errors.Join(ErrNeedsClearing, err)
	}

	return all, nil
}

// getUser returns an user matching the key or an error if the database is corrupted or no entry was found.
// Upon corruption, clearing the database is requested.
func getUser[K int | string](c *Cache, bucketName string, key K) (u userDB, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	err = c.db.View(func(tx *bbolt.Tx) error {
		bucket, err := getBucket(tx, bucketName)
		if err != nil {
			return errors.Join(ErrNeedsClearing, err)
		}

		u, err = getFromBucket[userDB](bucket, key)
		if err != nil {
			if !errors.Is(err, NoDataFoundError{}) {
				err = errors.Join(ErrNeedsClearing, err)
			}
			return err
		}

		return nil
	})

	if err != nil {
		return userDB{}, err
	}

	return u, nil
}
